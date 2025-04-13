package broadcast

import (
	"bytes"
	"fmt"
	log "github.com/sirupsen/logrus"
	"hash/crc32"
	"net"
	. "snow/common"
	"snow/config"
	"snow/internal/membership"
	"snow/tool"
	"time"
)

// 超时常量定义
var (
	DirectProbeTimeout = 1 * time.Second // 初始UDP探测超时
	IndirectAckTimeout = 3 * time.Second // 间接探测ACK处理器超时
	TCPFallbackTimeout = 3 * time.Second // TCP探测总超时
	IndirectProbeNum   = 3
)

// Ping 定义了心跳消息的格式
type Ping struct {
	Id int64 // 消息的序列号
}
type ForwardPing struct {
	Id     int64
	Target string //目标地址

}

// PingAck 定义了ack响应消息
type PingAckRes struct {
	Id         int64     // 消息序列号，与请求对应
	NodeState  NodeState // 节点当前状态
	IsIndirect bool      // 标识是中转确认还是目标响应
}
type server interface {
	HeartBeat(ip string, msg []byte)
	SendMessage(ip string, payload []byte, msg []byte)
	ConnectToPeer(addr string) (net.Conn, error)
	ReportLeave(ip []byte)
	KRandomNodes(k int, exclude []byte) []string
}
type udpServer interface {
	UDPSendMessage(remote string, payload, msg []byte) error
}

// Heartbeat 负责心跳消息的发送与探测
type Heartbeat struct {
	tool.ReentrantLock                            // 保护并发访问
	config             *config.Config             // 使用统一的配置
	member             *membership.MemberShipList // 直接使用MemberShipList
	server             server
	UdpServer          udpServer
	probeIndex         int  // 当前探测的索引
	running            bool // 服务是否在运行
	pingMap            *tool.CallbackMap
	stopCh             chan struct{} // 用来停止心跳探测循环
}

// NewHeartbeat 创建心跳服务
func NewHeartbeat(cfg *config.Config, memberList *membership.MemberShipList, server server, udpServer udpServer) *Heartbeat {
	// 生成随机初始序列号
	h := &Heartbeat{
		config:    cfg,
		member:    memberList,
		server:    server,
		UdpServer: udpServer,
		stopCh:    make(chan struct{}),
		pingMap:   tool.NewCallBackMap(),
	}
	return h
}

// Start 启动定时探测
func (h *Heartbeat) Start() {
	h.Lock()
	defer h.Unlock()

	if h.running {
		return
	}
	h.running = true
	go h.probeLoop()
}

// Stop 停止心跳探测
func (h *Heartbeat) Stop() {
	h.Lock()
	defer h.Unlock()
	if h.running {
		h.running = false
		close(h.stopCh)
		log.Infof("[INFO] Heartbeat service stopped")
	}
}

// probeLoop 心跳探测主循环
func (h *Heartbeat) probeLoop() {

	ticker := time.NewTicker(h.config.HeartbeatInterval)
	defer ticker.Stop()
	log.Debug("[debug] 2")

	for {
		select {
		case <-ticker.C:
			log.Debug("[debug] 1")
			h.probe()
		case <-h.stopCh:
			log.Infof("[INFO] Heartbeat probeLoop stopped")
			return
		}
	}
}

// probe 遍历 ipTable，选择下一个待探测的节点
func (h *Heartbeat) probe() {
	log.Debug("[debug]UDPServer running on ")

	length := h.member.MemberLen()
	if length <= 1 {
		return
	}
	numCheck := 0
	var target []byte
	localAddr := h.config.GetLocalAddr()
	h.Lock()
	for {
		h.member.Lock()
		length = h.member.MemberLen()
		if numCheck >= length {
			h.Unlock()
			log.Debugf("[DEBUG] heartbeat: No more nodes to check")
			return
		}
		if h.probeIndex >= length {
			h.probeIndex = 0
			numCheck++
			continue
		}
		if h.probeIndex < length {
			target = h.member.IPTable[h.probeIndex]
		}
		h.member.Unlock()
		// 跳过本机
		if bytes.Equal(localAddr, target) {
			h.probeIndex++
			numCheck++
			continue
		}
		h.probeIndex++
		break
	}
	h.Unlock()
	h.probeNode(target)
}

// probeNode 向目标节点发送 ping 心跳
func (h *Heartbeat) probeNode(addr []byte) {
	// 生成唯一的序列号
	id := h.nextId()
	p := Ping{
		Id: id,
	}
	targetAddr := tool.ByteToIPv4Port(addr)
	localAddr := h.config.ServerAddress
	log.Infof("[INFO] Node %s sending PING to %s (seq=%d)", localAddr, targetAddr, id)
	encode, err := tool.Encode(PingMsg, DirectPing, &p, false)
	if err != nil {
		log.Errorf("[ERR] Failed to encode ping message: %v", err)
		return
	}
	h.pingMap.Add(id, tool.ByteToIPv4Port(addr), func(ip string) {
		//超时就执行间接探测
		//h.server.HeartBeat(ip, encode)
		h.indirectProbe(ip)
	}, DirectProbeTimeout)

	err = h.UdpServer.UDPSendMessage(targetAddr, []byte{}, encode)
	if err != nil {
		log.Errorf("[ERR] Failed to send ping message to %s: %v", targetAddr, err)
		return
	}
}
func (h *Heartbeat) indirectProbe(ip string) {
	fmt.Println("[DEBUG] indirectProbe")
	data := ForwardPing{
		Id:     h.nextId(),
		Target: ip,
	}
	encode, err := tool.Encode(PingMsg, IndirectPing, data, false)
	if err != nil {
		log.Errorf("[ERR] Failed to send forwardPing message to %s: %v", ip, err)
		return
	}
	nodes := h.server.KRandomNodes(IndirectProbeNum, tool.IPv4To6Bytes(ip))
	for _, n := range nodes {
		h.UdpServer.UDPSendMessage(n, []byte{}, encode)
	}
}
func (h *Heartbeat) replayHeartbeat(ip string, id int64) {
	//fmt.Println("[DEBUG] replayHeartbeat")
	res := PingAckRes{
		Id:         id,
		IsIndirect: false,
	}
	encode, err := tool.Encode(PingMsg, PingAck, &res, false)
	if err != nil {
		log.Errorf("[ERR] Failed to encode ping message: %v", err)
		return
	}
	err = h.UdpServer.UDPSendMessage(ip, []byte{}, encode)
	if err != nil {
		log.Errorf("[ERR] Failed to send ack message: %v", err)
		return
	}
}

func (h *Heartbeat) nextId() int64 {
	h.Lock()
	defer h.Unlock()

	// 获取当前纳秒时间戳
	now := time.Now().UnixNano()

	// 使用服务器地址计算一个哈希值，确保不同节点生成不同的序列号
	addrHash := int64(crc32.ChecksumIEEE([]byte(h.config.ServerAddress)) % 10000)
	// 组合时间戳和地址哈希
	// 时间戳取低28位，再乘以10000，然后加上地址哈希
	// 这样即使在相同时间点，不同节点也会生成不同的序列号
	id := (int64(now&0x0FFFFFFF) * 10000) + addrHash

	// 更新结构体中的序列号字段
	return id
}

// Hand UDPServer的Hand方法实现，处理所有UDP消息
func (h *Heartbeat) Hand(msg []byte, conn net.Conn) {
	log.Println("UDPServer Hand")
	srcIp := conn.RemoteAddr().String()
	//目前udp只会发心跳类型的消息，第一位不用判断
	msgAction := msg[1]
	switch msgAction {
	case DirectPing:
		var ping Ping
		if err := tool.DecodeMsgPayload(msg, &ping); err != nil {
			log.Infof("Failed to decode ping message: %v", err)
			return
		}
		h.replayHeartbeat(srcIp, ping.Id)
	case PingAck:
		var ack PingAckRes
		if err := tool.DecodeMsgPayload(msg, &ack); err != nil {
			log.Infof("Failed to decode ack message: %v", err)
			return
		}
		h.pingMap.Delete(ack.Id)
	case IndirectPing:
		var forwardPing ForwardPing
		if err := tool.DecodeMsgPayload(msg, &forwardPing); err != nil {
			log.Infof("Failed to decode ack message: %v", err)
			return
		}
		p := Ping{
			Id: forwardPing.Id,
		}
		encode, err := tool.Encode(PingMsg, DirectPing, &p, false)
		if err != nil {
			log.Errorf("[ERR] Failed to encode ping message: %v", err)
			return
		}
		h.pingMap.Add(forwardPing.Id, forwardPing.Target, func(ip string) {
			//超时就执行
			fmt.Println("Indirect detection timeout")
			h.server.ReportLeave(tool.IPv4To6Bytes(ip))
		}, IndirectAckTimeout) //和发起节点保持一致
		err = h.UdpServer.UDPSendMessage(forwardPing.Target, []byte{}, encode)
		if err != nil {
			log.Error("Failed to forwardPing: %v", err)
		}
	}
}
