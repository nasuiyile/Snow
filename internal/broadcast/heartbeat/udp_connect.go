package heartbeat

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"snow/common"
	"snow/tool"
)

// UDPSendData 模拟 TCP 版本的 SendData，但记录的是 UDP 目标地址
type UDPSendData struct {
	RemoteAddr *net.UDPAddr // 目标 UDP 地址
	Header     []byte       // 消息头 (4字节长度等)
	Payload    []byte       // 主体负载
	Msg        []byte       // 额外消息
}

// UDPServer 封装一个简单的 UDP 服务逻辑
type UDPServer struct {
	conn      *net.UDPConn      // 绑定到本地的 UDP Socket
	localAddr *net.UDPAddr      // 本地监听地址
	stopCh    chan struct{}     // 控制关闭的通道
	sendCh    chan *UDPSendData // 待发送的消息队列
}

// NewUDPServer 创建并启动一个简单的 UDP 服务器
func NewUDPServer(localAddr string) (*UDPServer, error) {
	// 解析本地地址
	addr, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("ResolveUDPAddr failed: %v", err)
	}

	// 绑定地址，开始监听
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("ListenUDP failed: %v", err)
	}

	server := &UDPServer{
		conn:      conn,
		localAddr: addr,
		stopCh:    make(chan struct{}),
		sendCh:    make(chan *UDPSendData, 100), // 缓冲区大小 100 仅示例
	}

	log.Printf("UDPServer running on %s\n", localAddr)

	// 启动收包和发包的协程
	go server.startReading()
	go server.startSending()

	return server, nil
}

// startReading 不断从 conn 中读取数据包并进行处理
func (s *UDPServer) startReading() {
	defer log.Println("[UDPServer] reading loop exited")

	buf := make([]byte, 65535) // UDP 理论最大长度
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		// 阻塞读取数据包
		n, remoteAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("[UDPServer] ReadFromUDP error: %v", err)
			continue
		}
		data := buf[:n]

		// 简单演示：假设包最前 4 字节是 length，后面是正文
		if n < 4 {
			log.Printf("Packet from %s too small\n", remoteAddr)
			continue
		}
		header := data[:4]
		bodyLen := int(binary.BigEndian.Uint32(header))
		if bodyLen != (n - 4) {
			log.Printf("Packet length mismatch from %s: got %d, expected %d\n", remoteAddr, bodyLen, n-4)
			continue
		}
		body := data[4:]

		// 调用处理函数
		s.handlePacket(remoteAddr, header, body)
	}
}

// handlePacket 处理收到的包
func (s *UDPServer) handlePacket(remoteAddr *net.UDPAddr, header, body []byte) {
	log.Printf("Received from %s -> header=%v, body=%v\n", remoteAddr.String(), header, body)
	// 这里可以进一步做消息解析、业务逻辑等
}

// startSending 处理发送队列，将队列里的消息实际发送出去
func (s *UDPServer) startSending() {
	defer log.Println("[UDPServer] sending loop exited")
	for {
		select {
		case <-s.stopCh:
			return
		case data := <-s.sendCh:
			s.sendPacket(data)
		}
	}
}

// sendPacket 将 UDPSendData 中的数据拼装成一个完整的数据包并发送
func (s *UDPServer) sendPacket(d *UDPSendData) {
	// 先把 (payload + msg) 的长度写进 header
	totalLen := uint32(len(d.Payload) + len(d.Msg))
	binary.BigEndian.PutUint32(d.Header, totalLen)

	// 合并 header + payload + msg
	packet := append(d.Header, d.Payload...)
	packet = append(packet, d.Msg...)

	// 发送到目标地址
	n, err := s.conn.WriteToUDP(packet, d.RemoteAddr)
	if err != nil {
		log.Printf("[UDPServer] Error sending to %s: %v", d.RemoteAddr.String(), err)
		return
	}
	log.Printf("Sent %d bytes to %s\n", n, d.RemoteAddr.String())
}

// UDPSendMessage 对外提供的发送接口，类似 TCP 版的 SendMessage
func (s *UDPServer) UDPSendMessage(remote string, payload, msg []byte) {
	rAddr, err := net.ResolveUDPAddr("udp", remote)
	if err != nil {
		log.Printf("[UDPServer] Cannot resolve addr %s: %v", remote, err)
		return
	}
	header := make([]byte, 4) // 用来存储长度

	s.sendCh <- &UDPSendData{
		RemoteAddr: rAddr,
		Header:     header,
		Payload:    payload,
		Msg:        msg,
	}
}

// Close 优雅关闭服务
func (s *UDPServer) Close() {
	log.Println("[UDPServer] closing...")
	close(s.stopCh)
	err := s.conn.Close()
	if err != nil {
		return
	}
}

func SendUDPMessage(address [6]byte, msgType common.MsgType, msg interface{}) error {
	remoteStr := tool.ByteToIPv4Port(address[:])
	out, err := tool.Encode(msgType, msg, true)
	if err != nil {
		return err
	}

	udpAddr, err := net.ResolveUDPAddr("udp", remoteStr)
	if err != nil {
		return fmt.Errorf("failed to resolve udp addr %s: %v", remoteStr, err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return fmt.Errorf("failed to dial udp %s: %v", remoteStr, err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			log.Printf("[UDPServer] Failed to close connection: %v", cerr)
		}
	}()

	n, err := conn.Write(out.Bytes())
	if err != nil {
		return fmt.Errorf("failed to write msg to udp %s: %v", remoteStr, err)
	}
	log.Printf("[UDPServer] Successfully sent %d bytes to %s", n, remoteStr)
	return nil
}
