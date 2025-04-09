package broadcast

import (
	"bytes"
	log "github.com/sirupsen/logrus"
	"math/rand"
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

// AckResp 定义了更丰富的 ack 响应消息
type AckResp struct {
	SeqNo     int              // 消息序列号，与请求对应
	Timestamp int64            // 响应时间戳
	NodeState common.NodeState // 节点当前状态
	Source    []byte           // 响应来源节点信息
	Payload   []byte           // 自定义负载数据
}

// ackHandler 定义了处理 ACK 的回调机制
type ackHandler struct {
	ackFn   func(ackResp AckResp) // ACK 回调函数
	timeOut time.Time             // 超时时间
	timer   *time.Timer           // 超时定时器
}

// Suspect 定义了怀疑节点的信息
type Suspect struct {
	Incarnation int
	Addr        []byte // 被怀疑节点的地址
	Src         []byte // 发送怀疑信息的源地址
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
	}
	udpServer interface { // UDP服务器接口，用于发送UDP消息
		UDPSendMessage(remote string, payload, msg []byte) error
	}
	probeIndex  int                 // 当前探测的索引
	running     bool                // 服务是否在运行
	ackHandlers map[int]*ackHandler // 序列号到对应处理程序的映射
	stopCh      chan struct{}       // 增加 stopCh 用来停止心跳探测循环
}

// NewHeartbeat 创建心跳服务
func NewHeartbeat(
	cfg *config.Config,
	memberList *membership.MemberShipList,
	server interface {
		SendMessage(ip string, payload []byte, msg []byte)
		ConnectToPeer(addr string) (net.Conn, error)
		ReportLeave(ip []byte)
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

// Start 启动定时探测，每秒触发一次
func (h *Heartbeat) Start() {
	h.Lock()
	defer h.Unlock()

	if h.running {
		return
	}
	h.running = true

	go func() {
		h.probeLoop()
	}()
}

// Stop 停止心跳探测，退出 probeLoop
func (h *Heartbeat) Stop() {
	h.Lock()
	defer h.Unlock()
	if h.running {
		h.running = false
		close(h.stopCh)
		log.Infof("[INFO] Heartbeat service stopped")
	}
}

// probeLoop 心跳探测主循环（增加了 stopCh 的监听）
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
		log.Debug("[DEBUG] heartbeat: Received ACK from %s (seq=%d)",
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
		log.Error("[ERR] heartbeat: Failed to send UDP ping to %s: %v\n", tool.ByteToIPv4Port(addr[:]), err)
		h.handleRemoteFailure(addr, p)
		return
	}
	log.Debug("[UDPServer] Successfully sent from %s to %s", tool.ByteToIPv4Port(h.config.GetLocalAddr()), tool.ByteToIPv4Port(addr))

	// 等待 ACK 响应或超时
	select {
	case <-ackSuccess:
		// 成功收到 ACK
		return
	case <-time.After(h.config.HeartbeatInterval):
		// 超时，启动故障处理流程
		log.Debug("[DEBUG] heartbeat: ACK timeout for %s (seq=%d)", targetAddr, seqNo)
		h.handleRemoteFailure(addr, p)
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
			log.Debug("[DEBUG] heartbeat: ACK handler for seq=%d timed out", seqNo)
			delete(h.ackHandlers, seqNo)
		}
	})

	h.ackHandlers[seqNo] = handler
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

// handleRemoteFailure 当探测失败时调用，通过回调通知上层标记该节点为 suspect
func (h *Heartbeat) handleRemoteFailure(addr []byte, p Ping) {
	targetAddr := tool.ByteToIPv4Port(addr)
	log.Warnf("[WARN] heartbeat: No ack received from %s, initiating indirect probes\n", targetAddr)

	// 间接探测：选择 3 个随机节点，发送间接 pinglog.debug
	peers := h.selectKRandomNodes(h.config.IndirectChecks, addr)
	indirect := Ping{
		SeqNo: p.SeqNo,
		Addr:  addr,
		Src:   h.config.GetLocalAddr(),
	}

	// 设置间接探测的通道和超时
	indirectSuccess := make(chan struct{}, 1)
	indirectTimeout := time.After(5 * time.Second)

	// 注册间接 ACK 处理器
	h.registerAckHandler(p.SeqNo, func(ackResp AckResp) {
		log.Debug("[DEBUG] heartbeat: Received indirect ACK for %s", targetAddr)
		select {
		case indirectSuccess <- struct{}{}:
		default:
		}
	}, 1*time.Second)

	// 发送间接 UDP PING
	out, _ := tool.Encode(common.IndirectPingMsg, common.PingAction, &indirect, false)
	for _, peer := range peers {
		if err := h.udpServer.UDPSendMessage(tool.ByteToIPv4Port(peer), []byte{}, out); err != nil {
			log.Error("[ERR] heartbeat: Failed to send indirect ping to %s: %v\n", tool.ByteToIPv4Port(peer[:]), err)
		}
	}

	// 等待间接探测结果
	select {
	case <-indirectSuccess:
		log.Infof("[INFO] heartbeat: Node %s confirmed alive via indirect ping", targetAddr)
		return
	case <-indirectTimeout:
		// 间接探测超时，进行 TCP 后备探测
		log.Warnf("[Warnf] heartbeat: Indirect probes for %s timed out, attempting TCP fallback", targetAddr)

		// TCP 后备探测
		tcpSuccess := make(chan bool, 1)
		go func() {
			success := h.tcpFallbackProbe(addr, p)
			tcpSuccess <- success
		}()

		// 等待 TCP 探测结果
		select {
		case success := <-tcpSuccess:
			if success {
				log.Warnf("[WARN] heartbeat: TCP fallback succeeded for %s", targetAddr)
				return
			}
		case <-time.After(3 * time.Second):
			// TCP 探测也失败
			log.Infof("[INFO] heartbeat: Node %s timed out after TCP fallback", targetAddr)
		}

		// 标记节点为可疑
		log.Infof("[INFO] heartbeat: Node %s is suspected to have failed, no acks received\n", targetAddr)
		/*s := Suspect{
			Incarnation: 0,
			Addr:        addr,
			Src:         h.config.GetLocalAddr(),
		}
		h.markNodeSuspect(&s)*/
		h.server.ReportLeave(addr)
	}
}

// tcpFallbackProbe 使用TCP进行后备探测
func (h *Heartbeat) tcpFallbackProbe(addr []byte, p Ping) bool {
	targetAddr := tool.ByteToIPv4Port(addr)

	// 设置 TCP ACK 处理
	tcpSuccess := make(chan struct{})
	h.registerAckHandler(p.SeqNo, func(ackResp AckResp) {
		log.Debugf("[DEBUG] heartbeat: Received TCP ACK from %s", targetAddr)
		close(tcpSuccess)
	}, 3*time.Second)

	// 直接尝试发送消息，不需要重复建立连接
	out, _ := tool.Encode(common.PingMsg, common.PingAction, &p, false)
	h.server.SendMessage(targetAddr, []byte{}, out)
	log.Debugf("[DEBUG] heartbeat: Sent TCP fallback probe to %s, waiting for ACK...", targetAddr)

	// 等待 TCP ACK 或超时
	select {
	case <-tcpSuccess:
		return true
	case <-time.After(3 * time.Second):
		return false
	}
}

// nextSeqNo 返回下一个心跳消息的序列号（int 类型）
func (h *Heartbeat) nextSeqNo() int {
	h.Lock()
	defer h.Unlock()
	h.probeIndex++
	return h.probeIndex
}

// selectKRandomNodes 从 ipTable 中随机选择 k 个节点，排除指定的目标 addr
func (h *Heartbeat) selectKRandomNodes(k int, exclude []byte) [][]byte {
	h.Lock()
	defer h.Unlock()
	var candidates [][]byte
	for _, a := range h.memberList.IPTable {
		if tool.ByteToIPv4Port(a[:]) == tool.ByteToIPv4Port(exclude[:]) {
			continue
		}
		candidates = append(candidates, a)
	}
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	if len(candidates) > k {
		return candidates[:k]
	}
	return candidates
}

// markNodeSuspect 将节点标记为可疑
func (h *Heartbeat) markNodeSuspect(s *Suspect) {
	node := tool.ByteToIPv4Port(s.Addr)

	h.memberList.Lock()
	defer h.memberList.Unlock()

	meta, ok := h.memberList.MetaData[node]
	if !ok {
		return
	}

	if meta.State != common.NodeSurvival {
		return
	}

	// 如果是本节点被怀疑，发送反驳
	if node == h.config.ServerAddress {
		log.Warn("[WARN] Refuting suspect message for self")
		return
	}

	// 标记为可疑状态
	meta.State = common.NodeSuspected
	meta.UpdateTime = time.Now().Unix()

	// 广播可疑消息
	encode, _ := tool.Encode(common.SuspectMsg, common.NodeSuspected, s, false)
	h.server.SendMessage(node, []byte{}, encode)
}
