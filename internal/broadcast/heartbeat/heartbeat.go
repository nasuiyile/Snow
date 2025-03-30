package heartbeat

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"snow/common"
	"snow/internal/broadcast"
	"snow/tool"
	"time"
)

type Ping struct {
	SeqNo int    // 消息的序列号
	Addr  []byte // 目标地址（IP+端口）
	Src   []byte // 源地址（IP+端口）
}

// AckResp 定义了 ping 的 ack 响应消息
type ackResp struct {
	seqNo   int
	payload []byte
}

// indirectPingReq 定义了间接探测请求消息，精简后只保留目标地址、是否希望收到 nack 以及源地址信息
type indirectPingReq struct {
	seqNo int
	addr  []byte // 目标地址（IP+端口）
	nack  bool   // 如果为 true 表示希望收到 nack 响应
	src   []byte // 源地址（IP+端口）
}

// ackHandler 保存了用于处理 ack/nack 的回调函数和超时定时器
type ackHandler struct {
	ackFn  func(payload []byte, timestamp time.Time)
	nackFn func()
	timer  *time.Timer
}

type ackMessage struct {
	complete  bool
	payload   []byte
	timestamp time.Time
}

// Suspect 定义了怀疑节点的信息
type Suspect struct {
	Incarnation int
	Addr        []byte // 被怀疑节点的地址
	Src         []byte // 发送怀疑信息的源地址
}

// heartbeat 负责心跳消息的发送与探测
type heartbeat struct {
	tool.ReentrantLock     // 保护并发访问
	*broadcast.Server      // 使用指针引用 Server
	probeIndex         int // 当前探测的索引
	ackHandlers        map[int]*ackHandler
}

// NewHeartbeat 创建一个新的 heartbeat 实例，通过配置文件动态确定本机地址等信息
func NewHeartbeat(server *broadcast.Server) *heartbeat {
	hb := &heartbeat{
		Server: server, // 直接传递指针
	}
	// 启动定时探测
	hb.Start()
	return hb
}

func (h *heartbeat) getLocalAddr() []byte {
	ip := h.Server.Config.LocalAddress
	port := h.Server.Config.Port

	ipBytes := net.ParseIP(ip).To4()
	if ipBytes == nil {
		// 处理无效的 IP 地址格式
		log.Fatalf("Invalid IP address format: %s", ip)
	}
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))

	return append(ipBytes, portBytes...)
}

// getAdvertise 从配置中获取本机 IP 和端口
func (h *heartbeat) getAdvertise() (net.IP, int) {
	addr := h.getLocalAddr()
	ip := net.IP(addr[:4])
	port := int(binary.BigEndian.Uint16(addr[4:6]))
	return ip, port
}

// Start 启动定时探测，每秒触发一次
func (h *heartbeat) Start() {
	ticker := time.NewTicker(h.Server.Config.HeartbeatInterval)
	go func() {
		for range ticker.C {
			h.probe()
		}
	}()
}

// probe 遍历 ipTable，选择下一个待探测的节点
func (h *heartbeat) probe() {
	h.Lock()

	length := h.Member.MemberLen()
	if length == 0 {
		h.Unlock()
		return
	}

	numCheck := 0
	var target []byte
	localAddr := h.getLocalAddr()
	for {
		if numCheck >= length {
			h.Unlock()
			log.Println("[DEBUG] heartbeat: No more nodes to check, returning probe()")
			return
		}
		if h.probeIndex >= length {
			h.probeIndex = 0
			numCheck++
			continue
		}
		target = h.Server.Member.IPTable[h.probeIndex]
		// 跳过本机：比较配置生成的本机地址与目标地址
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
func (h *heartbeat) probeNode(addr []byte) {
	seqNo := h.nextSeqNo()
	p := Ping{
		SeqNo: seqNo,
		Addr:  addr,
		Src:   h.getLocalAddr(),
	}
	ackCh := make(chan ackMessage, h.Server.Config.IndirectChecks+1)
	nackCh := make(chan struct{}, h.Server.Config.IndirectChecks+1)
	h.setProbeChannels(seqNo, ackCh, nackCh, h.Server.Config.HeartbeatInterval)

	// 发送 UDP ping 消息
	if err := SendUDPMessage(addr, common.PingMsg, &p); err != nil {
		log.Printf("[ERR] heartbeat: Failed to send UDP ping to %s: %v\n", tool.ByteToIPv4Port(addr[:]), err)
		h.handleRemoteFailure(addr, p)
		return
	}
	log.Printf("[UDPServer] Successfully sent from %s to %s", tool.ByteToIPv4Port(h.getLocalAddr()), tool.ByteToIPv4Port(addr))

	select {
	case v := <-ackCh:
		if v.complete {
			// 收到 ack，无需进一步处理
			return
		}
		ackCh <- v
	case <-time.After(5 * time.Second):
		log.Printf("[DEBUG] heartbeat: UDP ping to %s timed out\n", tool.ByteToIPv4Port(addr[:]))
	}
	h.handleRemoteFailure(addr, p)
}

// handleRemoteFailure 当探测失败时调用，通过回调通知上层标记该节点为 suspect
func (h *heartbeat) handleRemoteFailure(addr []byte, p Ping) {
	log.Printf("[INFO] heartbeat: No ack received from %s, marking as suspect\n", tool.ByteToIPv4Port(addr[:]))

	// 间接探测：选择 3 个随机节点，发送间接 ping
	peers := h.selectKRandomNodes(h.Server.Config.IndirectChecks, addr)
	indirect := indirectPingReq{
		seqNo: p.SeqNo,
		addr:  addr,
		nack:  false,
		src:   h.getLocalAddr(),
	}
	for _, peer := range peers {
		if err := SendUDPMessage(peer, common.IndirectPingMsg, &indirect); err != nil {
			log.Printf("[ERR] heartbeat: Failed to send indirect ping to %s: %v\n", tool.ByteToIPv4Port(peer[:]), err)
		}
	}

	// TCP 后备探测
	fallbackCh := make(chan bool, 1)
	go func() {
		defer close(fallbackCh)
		didContact, err := h.sendPingAndWaitForAck(addr, p, time.Now().Add(1*time.Second))
		if err != nil {
			log.Printf("[ERR] heartbeat: TCP fallback ping failed to %s: %v\n", tool.ByteToIPv4Port(addr[:]), err)
		} else {
			fallbackCh <- didContact
		}
	}()
	select {
	case v := <-fallbackCh:
		if v {
			log.Printf("[WARN] heartbeat: TCP fallback succeeded for %s, but UDP had failed\n", tool.ByteToIPv4Port(addr[:]))
			return
		}
	case <-time.After(5 * time.Second):
		log.Printf("[INFO] heartbeat: Suspect %s has failed, no acks received\n", tool.ByteToIPv4Port(addr[:]))
		s := Suspect{
			Incarnation: 0,
			Addr:        addr,
			Src:         h.getLocalAddr(),
		}
		h.suspectNode(&s)
	}
}

// nextSeqNo 返回下一个心跳消息的序列号（int 类型）
func (h *heartbeat) nextSeqNo() int {
	h.Lock()
	defer h.Unlock()
	h.probeIndex++
	return h.probeIndex
}

// selectKRandomNodes 从 ipTable 中随机选择 k 个节点，排除指定的目标 addr
func (h *heartbeat) selectKRandomNodes(k int, exclude []byte) [][]byte {
	h.Lock()
	defer h.Unlock()
	var candidates [][]byte
	for _, a := range h.Server.Member.IPTable {
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

// setProbeChannels 设置用于探测的 ack/nack 回调和超时定时器
func (h *heartbeat) setProbeChannels(
	seqNo int,
	ackCh chan ackMessage,
	nackCh chan struct{},
	timeout time.Duration,
) {
	ackFn := func(payload []byte, timestamp time.Time) {
		select {
		case ackCh <- ackMessage{complete: true, payload: payload, timestamp: timestamp}:
		default:
		}
	}
	nackFn := func() {
		if nackCh == nil {
			return
		}
		select {
		case nackCh <- struct{}{}:
		default:
		}
	}
	ah := &ackHandler{
		ackFn:  ackFn,
		nackFn: nackFn,
	}
	h.Lock()
	if h.ackHandlers == nil {
		h.ackHandlers = make(map[int]*ackHandler)
	}
	h.ackHandlers[seqNo] = ah
	h.Unlock()
	ah.timer = time.AfterFunc(timeout, func() {
		h.Lock()
		if handler, ok := h.ackHandlers[seqNo]; ok && handler == ah {
			delete(h.ackHandlers, seqNo)
			select {
			case ackCh <- ackMessage{complete: false, payload: nil, timestamp: time.Now()}:
			default:
			}
		}
		h.Unlock()
	})
}

// sendPingAndWaitForAck 提供 TCP 后备探测功能，使用 SendMessage 发送 ping 请求
func (h *heartbeat) sendPingAndWaitForAck(address []byte, p Ping, deadline time.Time) (bool, error) {
	netAddrStr := tool.ByteToIPv4Port(address[:])

	// 创建 ping 消息
	out, err := tool.Encode(common.PingMsg, &p, false)
	h.Server.SendMessage(netAddrStr, []byte{}, out.Bytes()) // 使用 SendMessage 发送 ping 请求
	log.Printf("Successfully sent ping from %s to %s ", tool.ByteToIPv4Port(h.getLocalAddr()), netAddrStr)

	// 创建 ackHandler 和 ack/nack 通道
	ackCh := make(chan ackMessage, 1)
	nackCh := make(chan struct{}, 1)

	// 设置 ackHandler 来处理 ACK 和 NACK
	h.setProbeChannels(p.SeqNo, ackCh, nackCh, deadline.Sub(time.Now()))

	// 现在等待接收到的 ACK 响应
	conn, err := h.Server.ConnectToPeer(netAddrStr)
	if err != nil {
		log.Printf("Failed to connect to peer %s: %v\n", netAddrStr, err)
		return false, err
	}
	defer func(conn net.Conn) {
		if err := conn.Close(); err != nil {
			log.Printf("[ERR] heartbeat: Failed to close connection: %v\n", err)
		}
	}(conn)

	if err := conn.SetDeadline(deadline); err != nil {
		log.Printf("SetDeadline to %s error: %v\n", netAddrStr, err)
		return false, err
	}

	// 等待并处理收到的 ACK
	ackHeader := make([]byte, 4)
	if _, err := io.ReadFull(conn, ackHeader); err != nil {
		log.Printf("Error reading ack header from %s: %v\n", netAddrStr, err)
		nackCh <- struct{}{} // 发送 NACK
		return false, err
	}
	ackLen := binary.BigEndian.Uint32(ackHeader)
	if ackLen == 0 {
		nackCh <- struct{}{} // 发送 NACK
		return false, fmt.Errorf("ackLen is 0 from %s", netAddrStr)
	}

	ackBuf := make([]byte, ackLen)
	if _, err := io.ReadFull(bufio.NewReader(conn), ackBuf); err != nil {
		log.Printf("Error reading ack body from %s: %v\n", netAddrStr, err)
		nackCh <- struct{}{} // 发送 NACK
		return false, err
	}

	// 解码消息类型
	msgType, err := tool.DecodeMsgType(ackBuf)
	if err != nil {
		log.Printf("DecodeMsgType ack error from %s: %v\n", netAddrStr, err)
		nackCh <- struct{}{} // 发送 NACK
		return false, err
	}
	if msgType != common.AckRespMsg {
		nackCh <- struct{}{} // 发送 NACK
		return false, fmt.Errorf("expect AckRespMsg, got %d from %s", msgType, netAddrStr)
	}

	var ack ackResp
	if err = tool.DecodeMsgPayload(ackBuf, &ack); err != nil {
		log.Printf("Decode ackResp error from %s: %v\n", netAddrStr, err)
		nackCh <- struct{}{} // 发送 NACK
		return false, err
	}

	// 调用 ackHandler 的 ackFn
	ackCh <- ackMessage{complete: true, payload: ackBuf, timestamp: time.Now()}

	// 如果序列号匹配，返回 true
	if ack.seqNo == p.SeqNo {
		return true, nil
	}

	return false, fmt.Errorf("ack seqNo(%d) != ping seqNo(%d)", ack.seqNo, p.SeqNo)
}

func (h *heartbeat) suspectNode(s *Suspect) {
	h.Lock()
	defer h.Unlock()
	node := tool.ByteToIPv4Port(s.Addr)
	meta, ok := h.Server.Member.MetaData[node]
	if !ok {
		return
	}
	if meta.State != common.NodeSurvival {
		return
	}
	if node == tool.ByteToIPv4Port(h.getLocalAddr()) {
		log.Println("[WARN] Refuting suspect message for self from", s.Src)
		return
	}
	meta.State = common.NodeSuspected
	meta.UpdateTime = time.Now().Unix()
	encode, _ := tool.Encode(common.SuspectMsg, &s, false)
	h.Server.SendMessage(node, []byte{}, encode.Bytes())
	log.Println("[INFO] Marking", node, "as suspect (incarnation:", s.Incarnation, ") from:", s.Src)
}
