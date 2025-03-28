package heartbeat

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"snow/common"
	"snow/internal/broadcast"
	"snow/internal/dialer"
	"snow/tool"
	"time"
)

type Ping struct {
	SeqNo int     // 消息的序列号
	Addr  [6]byte // 目标地址（IP+端口）
	Src   [6]byte // 源地址（IP+端口）
}

// AckResp 定义了 ping 的 ack 响应消息
type ackResp struct {
	seqNo   int
	payload []byte
}

// indirectPingReq 定义了间接探测请求消息，精简后只保留目标地址、是否希望收到 nack 以及源地址信息
type indirectPingReq struct {
	seqNo int
	addr  [6]byte // 目标地址（IP+端口）
	nack  bool    // 如果为 true 表示希望收到 nack 响应
	src   [6]byte // 源地址（IP+端口）
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
	Addr        [6]byte // 被怀疑节点的地址
	Src         [6]byte // 发送怀疑信息的源地址
}

// heartbeat 负责心跳消息的发送与探测
type heartbeat struct {
	tool.ReentrantLock           // 保护并发访问
	ipTable            [][6]byte // 需要探测的节点列表
	probeIndex         int       // 当前探测的索引
	ackHandlers        map[int]*ackHandler
	config             broadcast.Config // 配置文件

	// suspectCallback 在心跳探测失败时被调用，用于通知上层更新节点状态
	suspectCallback func(s Suspect)
}

// NewHeartbeat 创建一个新的 heartbeat 实例，通过配置文件动态确定本机地址等信息
func NewHeartbeat(cfg *broadcast.Config) *heartbeat {
	return &heartbeat{
		ipTable:     make([][6]byte, 0),
		ackHandlers: make(map[int]*ackHandler),
		config:      *cfg,
	}
}

// SetIPTable 设置当前的 ipTable，外部模块可通过此方法更新需要探测的节点列表
func (h *heartbeat) SetIPTable(table [][6]byte) {
	h.Lock()
	defer h.Unlock()
	h.ipTable = table
}

// getLocalAddr 根据配置返回本机地址（转换为 [6]byte）
func (h *heartbeat) getLocalAddr() [6]byte {
	addr := tool.IPv4To6Bytes(h.config.DefaultAddress)
	return [6]byte(addr)
}

// getAdvertise 从配置中获取本机 IP 和端口
func (h *heartbeat) getAdvertise() (net.IP, int) {
	addr := h.getLocalAddr()
	ip := net.IPv4(addr[0], addr[1], addr[2], addr[3])
	port := int(binary.BigEndian.Uint16(addr[4:6]))
	return ip, port
}

// Start 启动定时探测，每秒触发一次
func (h *heartbeat) Start() {
	ticker := time.NewTicker(h.config.HeartbeatInterval)
	go func() {
		for range ticker.C {
			h.probe()
		}
	}()
}

// probe 遍历 ipTable，选择下一个待探测的节点
func (h *heartbeat) probe() {
	h.Lock()

	length := len(h.ipTable)
	if length == 0 {
		h.Unlock()
		return
	}

	numCheck := 0
	var target [6]byte
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
		target = h.ipTable[h.probeIndex]
		// 跳过本机：比较配置生成的本机地址与目标地址
		if tool.ByteToIPv4Port(localAddr[:]) == tool.ByteToIPv4Port(target[:]) {
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
func (h *heartbeat) probeNode(addr [6]byte) {
	seqNo := h.nextSeqNo()
	p := Ping{
		SeqNo: seqNo,
		Addr:  addr,
		Src:   h.getLocalAddr(),
	}
	ackCh := make(chan ackMessage, h.config.IndirectChecks+1)
	nackCh := make(chan struct{}, h.config.IndirectChecks+1)
	h.setProbeChannels(seqNo, ackCh, nackCh, h.config.HeartbeatInterval)

	// 发送 UDP ping 消息
	if err := SendUDPMessage(addr, common.PingMsg, &p); err != nil {
		log.Printf("[ERR] heartbeat: Failed to send UDP ping to %s: %v\n", tool.ByteToIPv4Port(addr[:]), err)
		h.handleRemoteFailure(addr, p)
		return
	}

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
func (h *heartbeat) handleRemoteFailure(addr [6]byte, p Ping) {
	log.Printf("[INFO] heartbeat: No ack received from %s, marking as suspect\n", tool.ByteToIPv4Port(addr[:]))

	// 间接探测：选择 3 个随机节点，发送间接 ping
	peers := h.selectKRandomNodes(h.config.IndirectChecks, addr)
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

	// TCP 后备探测：使用临时 TCP 端口进行 ping
	ip, _ := h.getAdvertise()
	clientAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:0", ip.String()))
	if err != nil {
		log.Printf("[ERR] heartbeat: Failed to resolve TCP client address: %v\n", err)
	}
	d := dialer.Dialer(clientAddr, h.config.TCPTimeout)
	fallbackCh := make(chan bool, 1)
	go func() {
		defer close(fallbackCh)
		didContact, err := sendPingAndWaitForAck(addr, p, time.Now().Add(1*time.Second), d)
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
		if h.suspectCallback != nil {
			h.suspectCallback(s)
		}
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
func (h *heartbeat) selectKRandomNodes(k int, exclude [6]byte) [][6]byte {
	h.Lock()
	defer h.Unlock()
	var candidates [][6]byte
	for _, a := range h.ipTable {
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

// sendPingAndWaitForAck 提供 TCP 后备探测功能（复用了 broadcast 的逻辑)
func sendPingAndWaitForAck(address [6]byte, p Ping, deadline time.Time, dialer net.Dialer) (bool, error) {
	netAddrStr := tool.ByteToIPv4Port(address[:])
	conn, err := dialer.Dial("tcp", netAddrStr)
	if err != nil {
		log.Printf("Failed to connect to peer %s: %v\n", netAddrStr, err)
		return false, err
	}
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			log.Printf("[ERR] heartbeat: Failed to close connection: %v\n", err)
		}
	}(conn)
	if err := conn.SetDeadline(deadline); err != nil {
		log.Printf("SetDeadline to %s error: %v\n", netAddrStr, err)
		return false, err
	}
	out, err := tool.Encode(common.PingMsg, &p, false)
	if err != nil {
		log.Printf("Encode ping to %s error: %v\n", netAddrStr, err)
		return false, err
	}
	length := int32(out.Len())
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(length))
	if _, err = conn.Write(header); err != nil {
		log.Printf("Error writing ping header to %s: %v\n", netAddrStr, err)
		return false, err
	}
	if _, err = conn.Write(out.Bytes()); err != nil {
		log.Printf("Error writing ping body to %s: %v\n", netAddrStr, err)
		return false, err
	}
	ackHeader := make([]byte, 4)
	if _, err := io.ReadFull(conn, ackHeader); err != nil {
		log.Printf("Error reading ack header from %s: %v\n", netAddrStr, err)
		return false, err
	}
	ackLen := binary.BigEndian.Uint32(ackHeader)
	if ackLen == 0 {
		return false, fmt.Errorf("ackLen is 0 from %s", netAddrStr)
	}
	ackBuf := make([]byte, ackLen)
	if _, err := io.ReadFull(bufio.NewReader(conn), ackBuf); err != nil {
		log.Printf("Error reading ack body from %s: %v\n", netAddrStr, err)
		return false, err
	}
	msgType, err := tool.DecodeMsgType(ackBuf)
	if err != nil {
		log.Printf("DecodeMsgType ack error from %s: %v\n", netAddrStr, err)
		return false, err
	}
	if msgType != common.AckRespMsg {
		return false, fmt.Errorf("expect ackRespMsg, got %d from %s", msgType, netAddrStr)
	}
	var ack ackResp
	if err = tool.DecodeMsgPayload(ackBuf, &ack); err != nil {
		log.Printf("Decode ackResp error from %s: %v\n", netAddrStr, err)
		return false, err
	}
	if ack.seqNo != p.SeqNo {
		return false, fmt.Errorf("ack seqNo(%d) != ping seqNo(%d)", ack.seqNo, p.SeqNo)
	}
	return true, nil
}

// SetSuspectCallback 设置怀疑节点回调函数
func (h *heartbeat) SetSuspectCallback(callback func(s Suspect)) {
	h.Lock()
	defer h.Unlock()
	h.suspectCallback = callback
}
