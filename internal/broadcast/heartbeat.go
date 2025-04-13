package broadcast

import (
	"bytes"
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
	DirectProbeTimeout   = 1 * time.Second  // 初始UDP探测超时
	IndirectProbeTimeout = 10 * time.Second // 间接探测等待超时
	IndirectAckTimeout   = 12 * time.Second // 间接探测ACK处理器超时
	TCPFallbackTimeout   = 12 * time.Second // TCP探测总超时
)

// Ping 定义了心跳消息的格式
type Ping struct {
	SeqNo int64  // 消息的序列号
	Addr  []byte // 目标地址（IP+端口）
	Src   []byte // 源地址（IP+端口）
}

// AckResp 定义了ack响应消息
type AckResp struct {
	SeqNo      int64     // 消息序列号，与请求对应
	Timestamp  int64     // 响应时间戳
	NodeState  NodeState // 节点当前状态
	Source     []byte    // 响应来源节点信息
	IsIndirect bool      // 标识是中转确认还是目标响应
}
type server interface {
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
		pingMap:   tool.NewCallBackMap(DirectProbeTimeout),
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

	for {
		select {
		case <-ticker.C:
			h.probe()
		case <-h.stopCh:
			log.Infof("[INFO] Heartbeat probeLoop stopped")
			return
		}
	}
}

// probe 遍历 ipTable，选择下一个待探测的节点
func (h *Heartbeat) probe() {
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
	seqNo := h.nextSeqNo()

	p := Ping{
		SeqNo: seqNo,
		Addr:  addr,
		Src:   h.config.GetLocalAddr(),
	}
	targetAddr := tool.ByteToIPv4Port(addr)
	localAddr := h.config.ServerAddress
	log.Infof("[INFO] Node %s sending PING to %s (seq=%d)", localAddr, targetAddr, seqNo)
	encode, err := tool.Encode(PingMsg, DirectPing, &p, false)
	if err != nil {
		log.Errorf("[ERR] Failed to encode ping message: %v", err)
		return
	}
	h.pingMap.Add(seqNo, tool.ByteToIPv4Port(addr), func(ip string) {
		//超时就执行间接探测
		h.indirectProbe(ip)
	})

	err = h.UdpServer.UDPSendMessage(targetAddr, []byte{}, encode)
	if err != nil {
		log.Errorf("[ERR] Failed to send ping message to %s: %v", targetAddr, err)
		return
	}
}
func (h *Heartbeat) indirectProbe(ip string) {
	log.Debug("[DEBUG] indirectProbe")
}

func (h *Heartbeat) nextSeqNo() int64 {
	h.Lock()
	defer h.Unlock()

	// 获取当前纳秒时间戳
	now := time.Now().UnixNano()

	// 使用服务器地址计算一个哈希值，确保不同节点生成不同的序列号
	addrHash := int64(crc32.ChecksumIEEE([]byte(h.config.ServerAddress)) % 10000)
	// 组合时间戳和地址哈希
	// 时间戳取低28位，再乘以10000，然后加上地址哈希
	// 这样即使在相同时间点，不同节点也会生成不同的序列号
	seqNo := (int64(now&0x0FFFFFFF) * 10000) + addrHash

	// 更新结构体中的序列号字段
	return seqNo
}

// Hand UDPServer的Hand方法实现，处理所有UDP消息
func (h *Heartbeat) Hand(msg []byte, conn net.Conn) {
	log.Println("UDPServer Hand")
	msgType := msg[0]
	//msgAction := msg[1]
	switch msgType {
	case PingMsg:
		var ping Ping
		if err := tool.DecodeMsgPayload(msg, &ping); err != nil {
			log.Infof("Failed to decode ping message: %v", err)
			return
		}
		//	switch msgType {
		//		sourceIP := tool.ByteToIPv4Port(ping.Src)
		//		localAddr := conn.LocalAddr().String()
		//		log.Infof("[INFO] Node %s 收到 PING 请求 (seq=%d) 来自 %s",
		//			localAddr,  // 接收方地址
		//			ping.SeqNo, // 序列号
		//			sourceIP) // 发送方地址
		//
		//		// 构造丰富的 ACK 响应
		//		ackResp := AckResp{
		//			SeqNo:      ping.SeqNo,
		//			Timestamp:  time.Now().UnixNano(),
		//			NodeState:  NodeSurvival, // 当前节点状态
		//			Source:     s.Config.IPBytes(),
		//			IsIndirect: false,
		//		}
		//
		//		log.Infof("[INFO] Node %s 准备回复 ACK (seq=%d) 给 %s",
		//			localAddr, ping.SeqNo, sourceIP)
		//
		//		// 发送 ACK 响应
		//		s.sendAckResponse(conn, ackResp, tool.ByteToIPv4Port(ping.Src))
		//
		//		// 更新发送方节点状态
		//		s.Member.Lock()
		//		meta, ok := s.Member.MetaData[sourceIP]
		//		if ok {
		//			meta.UpdateTime = time.Now().Unix()
		//			meta.State = NodeSurvival
		//		}
		//		s.Member.Unlock()
		//
		//	case IndirectPingMsg:
		//		var ping Ping
		//		if err := tool.DecodeMsgPayload(msg, &ping); err != nil {
		//			log.Debugf("[DEBUG] Failed to decode indirect ping message: %v", err)
		//			return
		//		}
		//
		//		targetAddr := tool.ByteToIPv4Port(ping.Addr)
		//		sourceAddr := tool.ByteToIPv4Port(ping.Src)
		//
		//		// 确认收到间接 PING 请求，设置 IsIndirect 为 true
		//		ackResp := AckResp{
		//			SeqNo:      ping.SeqNo,
		//			Timestamp:  time.Now().UnixNano(),
		//			NodeState:  NodeSurvival,
		//			Source:     s.Config.IPBytes(),
		//			IsIndirect: true,
		//		}
		//		s.sendAckResponse(conn, ackResp, sourceAddr)
		//
		//		// 转发ping到目标节点，使用当前节点作为源地址
		//		forwardPing := Ping{
		//			SeqNo: ping.SeqNo,
		//			Addr:  ping.Addr,
		//			Src:   s.Config.IPBytes(), // 使用当前节点的地址作为源
		//		}
		//
		//		forwardPingData, err := tool.Encode(PingMsg, PingAction, &forwardPing, false)
		//		if err != nil {
		//			log.Errorf("[ERR] Failed to encode forwardPing: %v", err)
		//			return
		//		}
		//
		//		// 检查 UDP Server 是否可用
		//		if s.UdpServer == nil {
		//			log.Warnf("[WARN] UDP Server not available, cannot forward ping to %s", targetAddr)
		//			return
		//		}
		//
		//		if err := s.UdpServer.UDPSendMessage(targetAddr, []byte{}, forwardPingData); err != nil {
		//			log.Errorf("[ERR] Failed to forward ping to %s: %v", targetAddr, err)
		//			return
		//		}
		//
		//		log.Printf("[INFO] Forward ping to %s", targetAddr)
		//
		//		// 注册回调处理目标节点的响应
		//		if s.HeartbeatService != nil {
		//			s.HeartbeatService.registerAckHandler(ping.SeqNo, func(targetAckResp AckResp) {
		//				// 创建转发的 ACK，设置 IsIndirect 为 false
		//				forwardAck := AckResp{
		//					SeqNo:      ping.SeqNo,
		//					Timestamp:  targetAckResp.Timestamp,
		//					NodeState:  targetAckResp.NodeState,
		//					Source:     targetAckResp.Source,
		//					IsIndirect: false,
		//				}
		//
		//				forwardAckData, err := tool.Encode(AckRespMsg, PingAction, &forwardAck, false)
		//				if err != nil {
		//					log.Errorf("[ERR] Failed to encode forwardAck: %v", err)
		//					return
		//				}
		//
		//				if err := s.UdpServer.UDPSendMessage(sourceAddr, []byte{}, forwardAckData); err != nil {
		//					log.Errorf("[ERR] Failed to forward ACK to %s: %v", sourceAddr, err)
		//					return
		//				}
		//
		//				log.Printf("[DEBUG] Successfully forwarded ACK from %s to %s (seq=%d)",
		//					targetAddr, sourceAddr, ping.SeqNo)
		//			}, 8*time.Second, nil)
		//		} else {
		//			log.Printf("[DEBUG] HeartbeatService not available, skipping ack handler registration")
		//		}
		//
		//		log.Printf("[DEBUG] Successfully forwarded ping to %s on behalf of %s", targetAddr, sourceAddr)
		//
		//	case AckRespMsg:
		//		// 处理 ACK 响应
		//		var ackResp AckResp
		//		if err := tool.DecodeMsgPayload(msg, &ackResp); err != nil {
		//			log.Debugf("[DEBUG] Failed to decode ack response: %v", err)
		//			return
		//		}
		//
		//		sourceAddr := tool.ByteToIPv4Port(ackResp.Source)
		//		log.Infof("[INFO] Node %s 收到 ACK 响应 (seq=%d) 来自 %s, isIndirect=%v",
		//			s.Config.ServerAddress, ackResp.SeqNo, sourceAddr, ackResp.IsIndirect)
		//
		//		// 更新节点状态
		//		s.Member.Lock()
		//		meta, ok := s.Member.MetaData[sourceAddr]
		//		if ok {
		//			meta.UpdateTime = time.Now().Unix()
		//			meta.State = ackResp.NodeState
		//			log.Infof("[INFO] 已更新节点 %s 的状态和时间戳, 序列号=%d", sourceAddr, ackResp.SeqNo)
		//		} else {
		//			log.Warnf("[WARN] 找不到节点 %s 的元数据, ACK seq=%d", sourceAddr, ackResp.SeqNo)
		//		}
		//		s.Member.Unlock()
		//
		//		// 如果心跳服务存在，通知它处理 ACK
		//		if s.HeartbeatService != nil {
		//			log.Infof("[INFO] 转发 ACK (seq=%d) 到心跳服务进行处理", ackResp.SeqNo)
		//			s.HeartbeatService.handleAckResponse(ackResp)
		//		} else {
		//			log.Warnf("[WARN] 心跳服务不可用，无法处理 ACK (seq=%d)", ackResp.SeqNo)
		//		}
		//	}
	}
}
