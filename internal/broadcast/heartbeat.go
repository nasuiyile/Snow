package broadcast

import (
	"bytes"
	log "github.com/sirupsen/logrus"
	"hash/crc32"
	"math/rand"
	"net"
	"snow/common"
	"snow/config"
	"snow/internal/membership"
	"snow/tool"
	"time"
)

// 超时常量定义
const (
	InitialProbeTimeout  = 8 * time.Second  // 初始UDP探测超时
	IndirectProbeTimeout = 10 * time.Second // 间接探测等待超时
	IndirectAckTimeout   = 12 * time.Second // 间接探测ACK处理器超时
	TCPFallbackTimeout   = 12 * time.Second // TCP探测总超时
)

// Ping 定义了心跳消息的格式
type Ping struct {
	SeqNo int    // 消息的序列号
	Addr  []byte // 目标地址（IP+端口）
	Src   []byte // 源地址（IP+端口）
}

// AckResp 定义了ack响应消息
type AckResp struct {
	SeqNo      int              // 消息序列号，与请求对应
	Timestamp  int64            // 响应时间戳
	NodeState  common.NodeState // 节点当前状态
	Source     []byte           // 响应来源节点信息
	IsIndirect bool             // 标识是中转确认还是目标响应
}

// ackHandler 定义了处理 ACK 的回调机制
type ackHandler struct {
	ackFn     func(ackResp AckResp) // ACK 回调函数
	timeOut   time.Time             // 超时时间
	timer     *time.Timer           // 超时定时器
	timeoutCh chan struct{}         // 超时通知通道
}

// Heartbeat 负责心跳消息的发送与探测
type Heartbeat struct {
	tool.ReentrantLock                            // 保护并发访问
	config             *config.Config             // 使用统一的配置
	memberList         *membership.MemberShipList // 直接使用MemberShipList
	server             interface {                // 服务器接口，用于发送TCP消息和建立连接
		SendMessage(ip string, payload []byte, msg []byte)
		ConnectToPeer(addr string) (net.Conn, error)
		ReportLeave(ip []byte)
		KRandomNodes(k int, exclude []byte) []string
	}
	udpServer interface { // UDP服务器接口，用于发送UDP消息
		UDPSendMessage(remote string, payload, msg []byte) error
	}
	probeIndex  int                 // 当前探测的索引
	seqNo       int                 // 序列号，用于跟踪心跳消息
	running     bool                // 服务是否在运行
	ackHandlers map[int]*ackHandler // 序列号+地址到对应处理程序的映射
	stopCh      chan struct{}       // 用来停止心跳探测循环
}

// NewHeartbeat 创建心跳服务
func NewHeartbeat(
	cfg *config.Config,
	memberList *membership.MemberShipList,
	server interface {
		SendMessage(ip string, payload []byte, msg []byte)
		ConnectToPeer(addr string) (net.Conn, error)
		ReportLeave(ip []byte)
		KRandomNodes(k int, exclude []byte) []string
	},
	udpServer interface {
		UDPSendMessage(remote string, payload, msg []byte) error
	}) *Heartbeat {
	// 生成随机初始序列号
	rand.Seed(time.Now().UnixNano())
	h := &Heartbeat{
		config:      cfg,
		memberList:  memberList,
		server:      server,
		udpServer:   udpServer,
		ackHandlers: make(map[int]*ackHandler),
		stopCh:      make(chan struct{}),
		seqNo:       0,
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
	h.Lock()
	length := h.memberList.MemberLen()
	if length <= 1 {
		h.Unlock()
		return
	}
	numCheck := 0
	var target []byte
	localAddr := h.config.GetLocalAddr()
	h.Lock()
	for {
		h.memberList.Lock()
		length = h.memberList.MemberLen()
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
			target = h.memberList.IPTable[h.probeIndex]
		}
		h.memberList.Unlock()
		if len(target) == 0 {
			h.Unlock()
			continue
		}
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

	ackSuccess := make(chan struct{})
	timeoutCh := make(chan struct{})
	timerCancelled := false

	// 注册 ACK 处理回调
	h.registerAckHandler(seqNo, func(ackResp AckResp) {
		// 加锁保护 timerCancelled 变量
		h.Lock()
		if timerCancelled {
			h.Unlock()
			return
		}
		timerCancelled = true
		h.Unlock()

		log.Infof("[INFO] Node %s received ACK from %s (seq=%d)",
			localAddr, targetAddr, seqNo)

		// 更新节点状态
		h.memberList.Lock()
		meta, ok := h.memberList.MetaData[targetAddr]
		if ok {
			meta.UpdateTime = time.Now().Unix()
			meta.State = ackResp.NodeState
		}
		h.memberList.Unlock()

		close(ackSuccess)
	}, InitialProbeTimeout, timeoutCh)

	// 发送 UDP ping 消息
	out, _ := tool.Encode(common.PingMsg, common.PingAction, &p, false)
	if err := h.udpServer.UDPSendMessage(targetAddr, []byte{}, out); err != nil {
		log.Errorf("[ERR] Node %s failed to send UDP ping to %s: %v",
			localAddr, targetAddr, err)

		// 取消定时器并清理处理器，避免后续不必要的超时
		h.Lock()
		timerCancelled = true
		delete(h.ackHandlers, seqNo)
		h.Unlock()

		h.checkNodeFailure(addr, p)
		return
	}

	// 等待 ACK 响应或超时，使用单一的超时机制
	select {
	case <-ackSuccess:
		// 成功收到 ACK，不需要做其他事情，因为回调已经处理了状态更新
		log.Infof("[INFO] Node %s successfully received ACK from %s (seq=%d)",
			localAddr, targetAddr, seqNo)
		return
	case <-timeoutCh:
		// 由处理器超时触发的通道
		h.Lock()
		if timerCancelled {
			h.Unlock()
			return
		}
		timerCancelled = true
		h.Unlock()

		log.Warnf("[WARN] Node %s waiting for ACK from %s timed out (seq=%d)",
			localAddr, targetAddr, seqNo)
		h.checkNodeFailure(addr, p)
	}
}

// registerAckHandler 注册一个 ACK 处理程序，使用复合键避免冲突
func (h *Heartbeat) registerAckHandler(seqNo int, callback func(AckResp), timeout time.Duration, timeoutCh chan struct{}) {
	h.Lock()
	defer h.Unlock()

	// 如果已存在处理器，记录日志
	if existHandler, exists := h.ackHandlers[seqNo]; exists {
		log.Warnf("[WARN] Node %s already has ACK handler for seq=%d, target=%s",
			h.config.ServerAddress, seqNo)

		// 清理旧处理器
		if existHandler.timer != nil {
			existHandler.timer.Stop()
		}
	}

	// 创建新的处理器
	timeoutTime := time.Now().Add(timeout)
	handler := &ackHandler{
		ackFn:     callback,
		timeOut:   timeoutTime,
		timeoutCh: timeoutCh,
	}

	h.ackHandlers[seqNo] = handler

	// 记录注册信息
	log.Infof("[INFO] Node %s registered ACK handler for seq=%d, timeout=%v",
		h.config.ServerAddress, seqNo, timeout)

	// 设置超时定时器，到期后自动清理处理器
	handler.timer = time.AfterFunc(timeout, func() {
		h.Lock()
		defer h.Unlock()

		// 检查处理程序是否仍然存在（可能已被处理过）
		if _, exists := h.ackHandlers[seqNo]; exists {
			log.Infof("[INFO] Node %s ACK handler for seq=%d, imed out and cleaned up",
				h.config.ServerAddress, seqNo)

			// 通知超时
			if handler.timeoutCh != nil {
				close(handler.timeoutCh)
			}

			delete(h.ackHandlers, seqNo)
		}
	})
}

// handleAckResponse 处理收到的 ACK 响应
func (h *Heartbeat) handleAckResponse(ackResp AckResp) {
	h.Lock()
	defer h.Unlock()

	sourceAddr := tool.ByteToIPv4Port(ackResp.Source)
	localAddr := h.config.ServerAddress
	seqNo := ackResp.SeqNo

	log.Infof("[INFO] Node %s handling ACK from %s (seq=%d, isIndirect=%v)",
		localAddr, sourceAddr, seqNo, ackResp.IsIndirect)

	handler, exists := h.ackHandlers[seqNo]
	if !exists {
		// 找不到处理器，可能是已经超时或者是重复的ACK
		log.Warnf("[WARN] Node %s no handler found for ACK from %s (seq=%d)",
			localAddr, sourceAddr, seqNo)

		// 打印活跃的处理器，帮助调试
		var activeHandlers []int
		for k := range h.ackHandlers {
			activeHandlers = append(activeHandlers, k)
		}
		if len(activeHandlers) > 0 {
			log.Infof("[INFO] Active handlers: %v", activeHandlers)
		}
		return
	}

	// 停止定时器，避免后续触发超时
	if handler.timer != nil {
		handler.timer.Stop()
	}

	// 调用回调函数
	if handler.ackFn != nil {
		log.Infof("[INFO] Node %s calling ACK handler for seq=%d from %s",
			localAddr, seqNo, sourceAddr)
		handler.ackFn(ackResp)
	}

	// 删除处理器
	delete(h.ackHandlers, seqNo)
}

// checkNodeFailure 检查节点故障，通过额外的探测方式确认节点是否真的下线
func (h *Heartbeat) checkNodeFailure(addr []byte, p Ping) {
	targetAddr := tool.ByteToIPv4Port(addr)
	log.Warnf("[WARN] heartbeat: No ack received from %s, initiating additional checks", targetAddr)

	// 间接探测：选择K个随机节点，让它们帮忙探测
	peers := h.server.KRandomNodes(h.config.IndirectChecks, addr)
	if len(peers) == 0 {
		log.Warnf("[WARN] heartbeat: No peers available for indirect probe, marking node %s as failed", targetAddr)
		h.reportNodeDown(addr)
		return
	}

	// 设置间接探测的通道
	indirectSuccess := make(chan struct{}, 1)
	timeoutCh := make(chan struct{})

	// 注册间接ACK处理
	h.registerAckHandler(p.SeqNo, func(ackResp AckResp) {
		if ackResp.IsIndirect {
			// 这是中转节点的确认，忽略
			log.Printf("[DEBUG] heartbeat: Received intermediate confirmation for %s, ignoring", targetAddr)
			return
		}

		// 这是目标节点的响应，表示目标节点存活
		log.Printf("[DEBUG] heartbeat: Received target node ACK for %s", targetAddr)
		select {
		case indirectSuccess <- struct{}{}:
		default:
		}
	}, IndirectAckTimeout, timeoutCh)

	for _, peer := range peers {
		// 发送间接探测请求 - 使用原有Ping结构体
		indirect := Ping{
			SeqNo: p.SeqNo,
			Addr:  addr,
			Src:   h.config.GetLocalAddr(),
		}
		out, _ := tool.Encode(common.IndirectPingMsg, common.PingAction, &indirect, false)
		if err := h.udpServer.UDPSendMessage(peer, []byte{}, out); err != nil {
			log.Errorf("[ERR] heartbeat: Failed to send indirect ping to %s: %v", peer, err)
			continue
		}
	}

	// 等待间接探测结果
	select {
	case <-indirectSuccess:
		log.Infof("[INFO]%s heartbeat: Node %s confirmed alive via indirect ping", h.config.ServerAddress, targetAddr)
		return
	case <-time.After(IndirectProbeTimeout):
		// 间接探测等待超时，尝试TCP探测作为最后尝试
		log.Warnf("[WARN] %s heartbeat: Indirect probes for %s timed out, attempting TCP fallback", h.config.ServerAddress, targetAddr)

		if h.doTCPFallbackProbe(addr, p) {
			log.Infof("[INFO] heartbeat: Node %s confirmed alive via TCP fallback", targetAddr)
			return
		}

		// 所有探测方式都失败，确认节点下线
		log.Warnf("[WARN] heartbeat: All probes failed for %s, marking node as DOWN", targetAddr)
		h.reportNodeDown(addr)
	}
}

// doTCPFallbackProbe 使用TCP进行后备探测
func (h *Heartbeat) doTCPFallbackProbe(addr []byte, p Ping) bool {
	targetAddr := tool.ByteToIPv4Port(addr)
	tcpSuccess := make(chan struct{})
	timeoutCh := make(chan struct{})

	// 注册TCP ACK处理
	h.registerAckHandler(p.SeqNo, func(ackResp AckResp) {
		log.Printf("[DEBUG] heartbeat: Received TCP ACK from %s", targetAddr)
		close(tcpSuccess)
	}, TCPFallbackTimeout, timeoutCh)

	// 使用TCP发送探测消息 - 保持使用Ping结构体
	out, _ := tool.Encode(common.PingMsg, common.PingAction, &p, false)
	h.server.SendMessage(targetAddr, []byte{}, out)
	log.Printf("[DEBUG] heartbeat: Sent TCP fallback probe to %s", targetAddr)

	// 等待TCP ACK响应，使用统一的超时时间
	select {
	case <-tcpSuccess:
		return true
	case <-time.After(TCPFallbackTimeout): // 使用统一常量
		log.Warnf("[WARN] heartbeat: TCP fallback probe timeout for %s", targetAddr)
		return false
	}
}

// reportNodeDown 报告节点已下线
func (h *Heartbeat) reportNodeDown(addr []byte) {
	targetAddr := tool.ByteToIPv4Port(addr)
	// 检查节点是否已经被标记为下线，避免重复广播
	h.memberList.Lock()
	meta, ok := h.memberList.MetaData[targetAddr]
	if !ok || meta.State == common.NodeLeft {
		h.memberList.Unlock()
		log.Debugf("[DEBUG] heartbeat: Node %s already marked as left, skipping", targetAddr)
		return
	}
	h.memberList.Unlock()

	// 直接广播节点离开消息
	log.Warnf("[WARN] heartbeat: Node %s failed all checks, broadcasting node leave", targetAddr)
	h.server.ReportLeave(addr)
}

func (h *Heartbeat) nextSeqNo() int {
	h.Lock()
	defer h.Unlock()

	// 获取当前纳秒时间戳
	now := time.Now().UnixNano()

	// 使用服务器地址计算一个哈希值，确保不同节点生成不同的序列号
	addrHash := int(crc32.ChecksumIEEE([]byte(h.config.ServerAddress)) % 10000)

	// 组合时间戳和地址哈希
	// 时间戳取低28位，再乘以10000，然后加上地址哈希
	// 这样即使在相同时间点，不同节点也会生成不同的序列号
	seqNo := (int(now&0x0FFFFFFF) * 10000) + addrHash

	// 更新结构体中的序列号字段
	h.seqNo = seqNo

	return seqNo
}
