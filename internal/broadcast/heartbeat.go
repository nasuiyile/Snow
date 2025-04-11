package broadcast

import (
	"bytes"
	log "github.com/sirupsen/logrus"
	"net"
	"snow/common"
	"snow/config"
	"snow/internal/membership"
	"snow/tool"
	"time"
)

// Ping 定义了心跳消息的格式
type Ping struct {
	SeqNo int    // 消息的序列号
	Addr  []byte // 目标地址（IP+端口）
	Src   []byte // 源地址（IP+端口）
}

// AckResp 定义了ack响应消息
type AckResp struct {
	SeqNo     int              // 消息序列号，与请求对应
	Timestamp int64            // 响应时间戳
	NodeState common.NodeState // 节点当前状态
	Source    []byte           // 响应来源节点信息
}

// ackHandler 定义了处理 ACK 的回调机制
type ackHandler struct {
	ackFn   func(ackResp AckResp) // ACK 回调函数
	timeOut time.Time             // 超时时间
	timer   *time.Timer           // 超时定时器
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
	running     bool                // 服务是否在运行
	ackHandlers map[int]*ackHandler // 序列号到对应处理程序的映射
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

	h := &Heartbeat{
		config:      cfg,
		memberList:  memberList,
		server:      server,
		udpServer:   udpServer,
		ackHandlers: make(map[int]*ackHandler),
		stopCh:      make(chan struct{}),
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
	if length == 0 {
		h.Unlock()
		return
	}

	numCheck := 0
	var target []byte
	localAddr := h.config.GetLocalAddr()
	for {
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
		h.memberList.Lock()
		if h.probeIndex < len(h.memberList.IPTable) {
			target = h.memberList.IPTable[h.probeIndex]
		}
		h.memberList.Unlock()
		if len(target) == 0 {
			h.Unlock()
			return
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
	seqNo := h.nextSeqNo()
	p := Ping{
		SeqNo: seqNo,
		Addr:  addr,
		Src:   h.config.GetLocalAddr(),
	}

	targetAddr := tool.ByteToIPv4Port(addr)
	ackSuccess := make(chan struct{})

	// 注册 ACK 处理回调
	h.registerAckHandler(seqNo, func(ackResp AckResp) {
		log.Debugf("[DEBUG] heartbeat: Received ACK from %s (seq=%d)",
			targetAddr, seqNo)

		// 更新节点状态
		h.memberList.Lock()
		meta, ok := h.memberList.MetaData[targetAddr]
		if ok {
			meta.UpdateTime = time.Now().Unix()
			meta.State = ackResp.NodeState
		}
		h.memberList.Unlock()

		close(ackSuccess)
	}, h.config.HeartbeatInterval)

	// 发送 UDP ping 消息
	out, _ := tool.Encode(common.PingMsg, common.PingAction, &p, false)
	if err := h.udpServer.UDPSendMessage(targetAddr, []byte{}, out); err != nil {
		log.Errorf("[ERR] heartbeat: Failed to send UDP ping to %s: %v", targetAddr, err)
		h.checkNodeFailure(addr, p)
		return
	}

	// 等待 ACK 响应或超时
	select {
	case <-ackSuccess:
		// 成功收到 ACK
		return
	case <-time.After(h.config.HeartbeatInterval):
		// 超时，启动故障处理流程
		log.Warnf("[WARN] heartbeat: ACK timeout for %s (seq=%d)", targetAddr, seqNo)
		h.checkNodeFailure(addr, p)
	}
}

// registerAckHandler 注册一个 ACK 处理程序
func (h *Heartbeat) registerAckHandler(seqNo int, callback func(AckResp), timeout time.Duration) {
	h.Lock()
	defer h.Unlock()

	// 如果已存在处理器，合并回调函数
	if existHandler, exists := h.ackHandlers[seqNo]; exists {
		log.Infof("[INFO] Merging ACK handlers for seq %d", seqNo)

		// 创建组合回调函数
		originalAckFn := existHandler.ackFn
		existHandler.ackFn = func(resp AckResp) {
			if originalAckFn != nil {
				originalAckFn(resp)
			}
			if callback != nil {
				callback(resp)
			}
		}
		return
	}

	timeoutTime := time.Now().Add(timeout)
	handler := &ackHandler{
		ackFn:   callback,
		timeOut: timeoutTime,
	}

	h.ackHandlers[seqNo] = handler

	// 设置超时定时器
	handler.timer = time.AfterFunc(timeout, func() {
		h.Lock()
		defer h.Unlock()

		// 检查处理程序是否仍然存在（可能已被处理过）
		if _, exists := h.ackHandlers[seqNo]; exists {
			log.Debugf("[DEBUG] heartbeat: ACK handler for seq=%d timed out", seqNo)
			delete(h.ackHandlers, seqNo)
		}
	})
}

// handleAckResponse 处理收到的 ACK 响应
func (h *Heartbeat) handleAckResponse(ackResp AckResp) {
	h.Lock()
	defer h.Unlock()

	handler, exists := h.ackHandlers[ackResp.SeqNo]
	if !exists {
		log.Debugf("[DEBUG] Multiple probes or late ACK for seq=%d", ackResp.SeqNo)
		return
	}

	if handler.timer != nil {
		handler.timer.Stop()
	}

	if handler.ackFn != nil {
		handler.ackFn(ackResp)
	}

	delete(h.ackHandlers, ackResp.SeqNo)
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

	// 设置间接探测的通道和超时
	indirectSuccess := make(chan struct{}, 1)
	indirectTimeout := time.After(2 * time.Second)

	// 注册间接ACK处理
	h.registerAckHandler(p.SeqNo, func(ackResp AckResp) {
		log.Debugf("[DEBUG] heartbeat: Received indirect ACK for %s", targetAddr)
		select {
		case indirectSuccess <- struct{}{}:
		default:
		}
	}, 2*time.Second)

	// 发送间接探测请求 - 使用原有Ping结构体
	indirect := Ping{
		SeqNo: p.SeqNo,
		Addr:  addr,
		Src:   h.config.GetLocalAddr(),
	}

	out, _ := tool.Encode(common.IndirectPingMsg, common.PingAction, &indirect, false)
	for _, peer := range peers {
		if err := h.udpServer.UDPSendMessage(peer, []byte{}, out); err != nil {
			log.Errorf("[ERR] heartbeat: Failed to send indirect ping to %s: %v", peer, err)
			continue
		}
	}

	// 等待间接探测结果
	select {
	case <-indirectSuccess:
		log.Infof("[INFO] heartbeat: Node %s confirmed alive via indirect ping", targetAddr)
		return
	case <-indirectTimeout:
		// 间接探测失败，尝试TCP探测作为最后尝试
		log.Warnf("[WARN] heartbeat: Indirect probes for %s timed out, attempting TCP fallback", targetAddr)

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

	// 注册TCP ACK处理
	h.registerAckHandler(p.SeqNo, func(ackResp AckResp) {
		log.Debugf("[DEBUG] heartbeat: Received TCP ACK from %s", targetAddr)
		close(tcpSuccess)
	}, 3*time.Second)

	// 使用TCP发送探测消息 - 保持使用Ping结构体
	out, _ := tool.Encode(common.PingMsg, common.PingAction, &p, false)
	h.server.SendMessage(targetAddr, []byte{}, out)
	log.Debugf("[DEBUG] heartbeat: Sent TCP fallback probe to %s", targetAddr)

	// 等待TCP ACK响应
	select {
	case <-tcpSuccess:
		return true
	case <-time.After(3 * time.Second):
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

// nextSeqNo 返回下一个心跳消息的序列号
func (h *Heartbeat) nextSeqNo() int {
	h.Lock()
	defer h.Unlock()
	h.probeIndex++
	return h.probeIndex
}
