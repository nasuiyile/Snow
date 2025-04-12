package broadcast

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"net"
	"snow/common"
	"snow/config"
	"snow/internal/dialer"
	"snow/internal/membership"
	"snow/internal/state"
	"snow/tool"
	"strings"
	"time"
)

// 定义一个结构体来封装发送的数据
type SendData struct {
	Conn    net.Conn
	Header  []byte
	Payload []byte
	Msg     []byte
}

// NewServer 创建并启动一个 TCP 服务器
func NewServer(config *config.Config, action Action) (*Server, error) {
	if config.Test {
		tool.DebugLog()
	}
	listener, err := net.Listen("tcp", config.ServerAddress)
	if err != nil {
		return nil, err
	}
	clientAddress, err := net.ResolveTCPAddr("tcp", config.ClientAddress)
	if err != nil {
		return nil, err
	}
	server := &Server{
		listener: listener,
		Config:   config,
		Member: membership.MemberShipList{
			IPTable:  make([][]byte, 0),
			MetaData: make(map[string]*membership.MetaData),
		},
		State: state.State{
			State:           state.NewTimeoutMap(),
			ReliableTimeout: make(map[string]*state.ReliableInfo),
		},
		Action:   action,
		client:   dialer.Dialer(clientAddress, config.TCPTimeout),
		StopCh:   make(chan struct{}),
		SendChan: make(chan *SendData),
	}
	server.IsClosed.Store(false)
	server.H = server
	server.Member.FindOrInsert(config.IPBytes())
	for _, addr := range config.DefaultServer {
		server.Member.AddMember(tool.IPv4To6Bytes(addr), common.NodeSurvival)
	}
	if server.Config.HeartBeat && server.Config.Test {
		server.StartHeartBeat()
	}

	go server.startAcceptingConnections() // 启动接受连接的协程

	server.schedule()
	//server.ApplyJoin(config.InitialServer)
	fmt.Printf("Server is running on port %d...\n\n", config.Port)
	return server, nil

}

func (s *Server) StartHeartBeat() {
	// 初始化UDP服务
	udpServer, err := NewUDPServer(s.Config)
	if err != nil {
		log.Warn("[WARN] Failed to initialize UDP server: %v", err)
	} else {
		s.UdpServer = udpServer
		udpServer.H = s
	}

	// 初始化Heartbeat服务 - 直接传递server和udpServer
	s.HeartbeatService = NewHeartbeat(
		s.Config,
		&s.Member,
		s,
		s.UdpServer,
	)
	s.HeartbeatService.Start()
}

func (s *Server) schedule() {
	// Create the stop tick channel, a blocking channel. We close this
	// when we should stop the tickers.
	//go s.pushTrigger(s.StopCh)
	go s.Sender()

}

// startAcceptingConnections 不断接受新的客户端连接
func (s *Server) startAcceptingConnections() {
	for {
		select {
		case <-s.StopCh:
			return
		default:
		}
		//其实是由netpoller 来触发的
		conn, err := s.listener.Accept()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				log.Infof("Listener closed, stopping connection acceptance.")
				return
			}
			log.Errorf("Error accepting connection:", err)
			continue
		}
		//log.Printf("get connect from %s", conn.RemoteAddr().String())
		tcpConn := conn.(*net.TCPConn)
		tcpConn.SetLinger(0)
		conn = tcpConn
		serverIp := s.Config.GetServerIp(conn.RemoteAddr().String())
		metaData := membership.NewEmptyMetaData()
		metaData.SetServer(conn)
		s.Member.PutMemberIfNil(serverIp, metaData)

		// todo 后期优化成只能保持k个连接
		go s.handleConnection(conn, false) // 处理客户端连接

	}
}

// handleConnection 处理客户端连接
func (s *Server) handleConnection(conn net.Conn, isServer bool) {
	defer func() {
		safeCloseConnection(conn)
		addr := conn.RemoteAddr().String()
		s.Member.Lock()
		if !isServer {
			addr = s.Config.GetServerIp(addr)
		}
		s.Member.RemoveMember(tool.IPv4To6Bytes(addr), false)
		s.Member.Unlock()
	}()
	reader := bufio.NewReader(conn)

	for {
		select {
		case <-s.StopCh:
			return
		default:
		}

		// 读取消息头 (4字节表示消息长度)

		header := make([]byte, 4)
		_, err := io.ReadFull(reader, header)
		if err != nil {
			log.Errorf("Read header error from %v: %v\n", conn.RemoteAddr(), err)
			if err == io.EOF {
				log.Warn("Normal EOF: connection closed by client")
			}
			member := tool.IPv4To6Bytes(conn.RemoteAddr().String())
			s.ReportLeave(member)
			//s.Member.RemoveMember(member, false)
			return
		}

		// 解析消息长度
		messageLength := int(binary.BigEndian.Uint32(header))
		if messageLength <= 0 {
			log.Errorf("Invalid message length from %v: %d\n", conn.RemoteAddr(), messageLength)
			continue
		}
		// 根据消息长度读取消息体
		msg := make([]byte, messageLength)
		_, err = io.ReadFull(reader, msg)
		if err != nil {
			log.Errorf("Read body error from %v: %v\n", conn.RemoteAddr(), err)
			return
		}
		s.H.Hand(msg, conn)

	}
}

func (s *Server) ConnectToPeer(addr string) (net.Conn, error) {
	member := s.Member.GetOrPutMember(addr)
	member.Lock()
	defer member.Unlock()
	if member.GetClient() != nil {
		return member.GetClient(), nil
	}
	// 赋值给 Dialer 的 LocalAddr
	conn, err := s.client.Dial("tcp", addr)
	if err != nil {
		log.Warnf("Failed to connect to %s: %v\n", addr, err)
		return nil, err
	}
	//log.Debugff("%sConnected to %s\n", s.Config.ServerAddress, addr)
	member.SetClient(conn)
	return conn, nil
}
func (s *Server) SendMessage(ip string, payload []byte, msg []byte) {
	if s.IsClosed.Load() {
		return
	}
	go func() {
		metaData := s.Member.GetOrPutMember(ip)
		var conn net.Conn
		conn = metaData.GetClient()
		if conn == nil {
			//先建立一次链接进行尝试
			newConn, err := s.ConnectToPeer(ip)
			if err != nil {
				log.Error(s.Config.ServerAddress, "can't connect to ", ip)
				// 避免递归调用导致的堆栈增长
				if !s.IsClosed.Load() {
					s.ReportLeave(tool.IPv4To6Bytes(ip))
				}
				return
			} else {
				conn = newConn
			}
		} else if !isConnectionAlive(conn) {
			// 连接存在但无效，尝试重新连接
			safeCloseConnection(conn)

			newConn, err := s.ConnectToPeer(ip)
			if err != nil {
				log.Errorf("[ERROR] %s can't reconnect to %s: %v", s.Config.ServerAddress, ip, err)
				if !s.IsClosed.Load() {
					s.ReportLeave(tool.IPv4To6Bytes(ip))
				}
				return
			}
			conn = newConn
		}
		// 创建消息头，存储消息长度 (4字节大端序)
		length := uint32(len(payload) + len(msg))

		header := make([]byte, 4)
		binary.BigEndian.PutUint32(header, length)
		data := &SendData{
			Conn:    conn,
			Header:  header,
			Payload: payload,
			Msg:     msg,
		}
		s.SendChan <- data
	}()

}

// 检查连接是否有效
func isConnectionAlive(conn net.Conn) bool {
	if conn == nil {
		return false
	}

	// 设置一个非常短的读取超时
	err := conn.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
	if err != nil {
		return false
	}

	// 尝试读取一个字节，但不期望有数据
	one := make([]byte, 1)
	if _, err := conn.Read(one); err != nil {
		if err == io.EOF || strings.Contains(err.Error(), "use of closed network connection") {
			return false // 连接已关闭
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			// 重置读取超时
			_ = conn.SetReadDeadline(time.Time{})
			return true // 超时但连接可能仍然有效
		}
		return false
	}

	// 重置读取超时
	_ = conn.SetReadDeadline(time.Time{})
	return true
}

// safeCloseConnection 安全地关闭一个TCP连接
func safeCloseConnection(conn net.Conn) {
	if conn == nil {
		return
	}

	// 尝试转换为TCP连接，设置Linger为0表示立即关闭
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		// 设置SO_LINGER选项为0，使Close()立即返回，操作系统会发送RST而不是FIN
		err := tcpConn.SetLinger(0)
		if err != nil {
			return
		}

		// 关闭读写
		_ = tcpConn.CloseRead()
		_ = tcpConn.CloseWrite()
	}

	// 关闭连接，忽略可能的错误
	_ = conn.Close()
}

// 这个方法只能用来回复消息
func (s *Server) replayMessage(conn net.Conn, msg []byte) {
	// 创建消息头，存储消息长度 (4字节大端序)
	length := uint32(len(msg))
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, length)
	log.Errorf(conn.RemoteAddr().String())
	data := &SendData{
		Conn:    conn,
		Header:  header,
		Payload: []byte{},
		Msg:     msg,
	}
	s.SendChan <- data
}

// Close 关闭服务器
func (s *Server) Close() {
	s.Member.Lock()
	defer s.Member.Unlock()
	if s.IsClosed.Load() {
		return
	}
	s.IsClosed.Store(true)

	for _, v := range s.Member.MetaData {
		client := v.GetClient()
		if client != nil {
			safeCloseConnection(client)
			v.SetClient(nil)

		}
		server := v.GetServer()
		if server != nil {
			safeCloseConnection(client)
			v.SetClient(nil)
		}
	}
	if s.HeartbeatService != nil {
		s.HeartbeatService.Stop()
	}
	if s.UdpServer != nil {
		s.UdpServer.Close()
	}
	s.listener.Close()

}

func (s *Server) Sender() {
	for data := range s.SendChan {
		data := data
		go func() {
			err := data.Conn.SetWriteDeadline(time.Now().Add(s.Config.TCPTimeout))
			if err != nil {
				log.Error(err)
			}
			if s.Config.Test && s.Config.Report {
				bytes := append(data.Payload, data.Msg...)
				tool.SendHttp(s.Config.ServerAddress, data.Conn.RemoteAddr().String(), bytes, s.Config.FanOut)
			}
			_, err = data.Conn.Write(data.Header)
			if err != nil {
				log.Errorf("Error sending header to %v: %v", data.Conn.RemoteAddr(), err)
				s.ReportLeave(tool.IPv4To6Bytes(data.Conn.RemoteAddr().String()))
				return
			}
			_, err = data.Conn.Write(data.Payload)
			if err != nil {
				s.ReportLeave(tool.IPv4To6Bytes(data.Conn.RemoteAddr().String()))
				log.Errorf("Error sending payload to %v: %v", data.Conn.RemoteAddr(), err)
				return
			}
			_, err = data.Conn.Write(data.Msg)
			if err != nil {
				s.ReportLeave(tool.IPv4To6Bytes(data.Conn.RemoteAddr().String()))
				log.Errorf("Error sending message to %v: %v", data.Conn.RemoteAddr(), err)
				return
			}
		}()

	}
}
