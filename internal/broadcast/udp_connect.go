package broadcast

import (
	"encoding/binary"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net"
	"snow/common"
	"snow/config"
	"time"
)

// UDPSendData 定义了待发送 UDP 消息的数据结构
type UDPSendData struct {
	RemoteAddr *net.UDPAddr // 目标 UDP 地址
	Header     []byte       // 消息头 (4 字节表示负载长度)
	Payload    []byte       // 消息主体负载
	Msg        []byte       // 额外消息
}

// UDPServer 封装了 UDP 服务器功能
type UDPServer struct {
	conn      *net.UDPConn       // UDP 连接（监听 socket）
	localAddr *net.UDPAddr       // 本地监听地址
	stopCh    chan struct{}      // 用于停止服务的通道
	sendCh    chan *UDPSendData  // 发送队列
	Config    *config.Config     // 配置参数
	H         common.HandlerFunc // H 是消息处理回调
}

// NewUDPServer 创建并启动一个 UDP 服务器
func NewUDPServer(config *config.Config) (*UDPServer, error) {
	// 解析本地地址
	addr, err := net.ResolveUDPAddr("udp", config.ServerAddress)
	if err != nil {
		return nil, fmt.Errorf("ResolveUDPAddr failed: %v", err)
	}

	// 绑定地址并开始监听
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("ListenUDP failed: %v", err)
	}

	server := &UDPServer{
		conn:      conn,
		localAddr: addr,
		stopCh:    make(chan struct{}),
		sendCh:    make(chan *UDPSendData, 100),
		Config:    config,
	}
	server.H = server

	log.Info("UDPServer running on %s\n", config.ServerAddress)

	// 启动读取和发送消息的协程
	go server.startReading()
	go server.startSending()

	return server, nil
}

// udpConnWrapper 用于将 UDP 的连接和目标地址包装为 net.Conn 接口实现
type udpConnWrapper struct {
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
}

func (u *udpConnWrapper) Read(b []byte) (int, error) {
	// UDP 是无连接的，通常不使用 Read，此处返回错误
	return 0, fmt.Errorf("read not supported on udpConnWrapper")
}

func (u *udpConnWrapper) Write(b []byte) (int, error) {
	return u.conn.WriteToUDP(b, u.remoteAddr)
}

func (u *udpConnWrapper) Close() error {
	return nil
}

func (u *udpConnWrapper) LocalAddr() net.Addr {
	return u.conn.LocalAddr()
}

func (u *udpConnWrapper) RemoteAddr() net.Addr {
	return u.remoteAddr
}

func (u *udpConnWrapper) SetDeadline(t time.Time) error {
	return u.conn.SetDeadline(t)
}

func (u *udpConnWrapper) SetReadDeadline(t time.Time) error {
	return u.conn.SetReadDeadline(t)
}

func (u *udpConnWrapper) SetWriteDeadline(t time.Time) error {
	return u.conn.SetWriteDeadline(t)
}

// startReading 循环读取 UDP 消息
func (s *UDPServer) startReading() {
	defer log.Info("[UDPServer] Reading loop exited")

	buf := make([]byte, 65535) // UDP 理论最大长度
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		n, remoteAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			log.Error("[UDPServer] ReadFromUDP error: %v", err)
			// 如果错误不是临时错误（例如连接已关闭），则退出循环
			var ne net.Error
			if errors.As(err, &ne) && !ne.Temporary() {
				return
			}
			continue
		}

		// 至少需要 4 字节 header
		if n < 4 {
			log.Warn("Received packet too short from %s: %d bytes", remoteAddr.String(), n)
			continue
		}

		data := buf[:n]
		header := data[:4]
		bodyLen := int(binary.BigEndian.Uint32(header))
		if bodyLen != n-4 {
			log.Warn("Packet length mismatch from %s: got %d, expected %d", remoteAddr.String(), bodyLen, n-4)
			continue
		}
		body := data[4:]

		// 调用处理函数 Hand，将 body 作为消息内容，
		// 并传入一个 UDPConnWrapper 将 UDP 连接包装为 net.Conn 传入
		wrapper := &udpConnWrapper{
			conn:       s.conn,
			remoteAddr: remoteAddr,
		}
		s.H.Hand(body, wrapper)
	}
}

// sendUDPReply 发送 UDP 回复消息，默认回复 "UDP_ACK"
func (s *UDPServer) sendUDPReply(addr *net.UDPAddr, msg []byte) {
	replyBody := []byte("UDP_ACK")
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(replyBody)))
	packet := append(header, replyBody...)
	_, err := s.conn.WriteToUDP(packet, addr)
	if err != nil {
		log.Error("[UDPServer] Error sending to %s: %v", addr.String(), err)
		return
	}
	log.Debug("[UDPServer] Sent ACK to %s", addr.String())
}

// startSending 处理发送队列，将消息实际发送出去
func (s *UDPServer) startSending() {
	defer log.Info("[UDPServer] Sending loop exited")
	for {
		select {
		case <-s.stopCh:
			return
		case data := <-s.sendCh:
			s.sendPacket(data)
		}
	}
}

// sendPacket 将 UDPSendData 中的数据拼装为完整数据包并发送
func (s *UDPServer) sendPacket(d *UDPSendData) {
	// 拼接 header + payload + msg
	packet := append(d.Header, d.Payload...)
	packet = append(packet, d.Msg...)
	_, err := s.conn.WriteToUDP(packet, d.RemoteAddr)
	if err != nil {
		log.Error("[UDPServer] Error sending to %s: %v", d.RemoteAddr.String(), err)
		return
	}
}

// UDPSendMessage 封装发送 UDP 消息的接口
func (s *UDPServer) UDPSendMessage(remote string, payload, msg []byte) error {
	rAddr, err := net.ResolveUDPAddr("udp", remote)
	if err != nil {
		log.Error("[UDPServer] Cannot resolve addr %s: %v", remote, err)
		return fmt.Errorf("failed to resolve address %s: %v", remote, err)
	}

	totalLen := uint32(len(payload) + len(msg))
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, totalLen)

	data := &UDPSendData{
		RemoteAddr: rAddr,
		Header:     header,
		Payload:    payload,
		Msg:        msg,
	}
	s.sendCh <- data
	return nil
}

// Close 优雅关闭 UDP 服务
func (s *UDPServer) Close() {
	log.Info("[UDPServer] Closing...")
	close(s.stopCh)
	if err := s.conn.Close(); err != nil {
		log.Error("Error closing UDP connection: %v", err)
	}
}
