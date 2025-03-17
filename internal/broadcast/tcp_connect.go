package broadcast

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"snow/internal/dialer"
	"snow/internal/membership"
	"snow/internal/state"
	"snow/tool"
)

// 定义一个结构体来封装发送的数据
type SendData struct {
	Conn    net.Conn
	Header  []byte
	Payload []byte
	Msg     []byte
}

//var stopCh = make(chan struct{})
//var sendChan = make(chan *SendData)
//var clientWorkerPool = tool.NewWorkerPool(1)

// NewServer 创建并启动一个 TCP 服务器
func NewServer(config *Config, action Action) (*Server, error) {
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
		Action:           action,
		client:           dialer.Dialer(clientAddress, config.TCPTimeout),
		isClosed:         false,
		stopCh:           make(chan struct{}),
		sendChan:         make(chan *SendData),
		clientWorkerPool: tool.NewWorkerPool(1),
	}

	server.Member.FindOrInsert(config.IPBytes())
	go server.startAcceptingConnections() // 启动接受连接的协程

	//for _, addr := range config.DefaultServer {
	//	server.Member.AddMember(tool.IPv4To6Bytes(addr))
	//}
	server.schedule()
	server.ApplyJoin(config.InitialServer)
	log.Printf("Server is running on port %d...\n\n", config.Port)
	return server, nil

}

func (s *Server) schedule() {
	// Create the stop tick channel, a blocking channel. We close this
	// when we should stop the tickers.
	go s.pushTrigger(s.stopCh)
	go s.Sender()

}

// startAcceptingConnections 不断接受新的客户端连接
func (s *Server) startAcceptingConnections() {
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}
		//其实是由netpoller 来触发的
		conn, err := s.listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		log.Printf("get connect from %s", conn.RemoteAddr().String())
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
		addr := conn.RemoteAddr().String()
		s.Member.Lock()
		if !isServer {
			addr = s.Config.GetServerIp(addr)
		}
		s.Member.RemoveMember(tool.IPv4To6Bytes(addr))
		s.Member.Unlock()
	}()
	reader := bufio.NewReader(conn)

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		// 读取消息头 (4字节表示消息长度)

		header := make([]byte, 4)
		_, err := io.ReadFull(reader, header)
		if err != nil {
			fmt.Println(errors.Is(err, io.EOF))
			log.Printf("Read header error from %v: %v\n", conn.RemoteAddr(), err)
			if err == io.EOF {
				fmt.Println("Normal EOF: connection closed by client")
			}
			fmt.Println(conn.RemoteAddr().String())
			s.Member.RemoveMember(tool.IPv4To6Bytes(conn.RemoteAddr().String()))
			return
		}

		// 解析消息长度
		messageLength := int(binary.BigEndian.Uint32(header))
		if messageLength <= 0 {
			log.Printf("Invalid message length from %v: %d\n", conn.RemoteAddr(), messageLength)
			continue
		}
		// 根据消息长度读取消息体
		msg := make([]byte, messageLength)
		_, err = io.ReadFull(reader, msg)
		if err != nil {
			log.Printf("Read body error from %v: %v\n", conn.RemoteAddr(), err)
			return
		}

		handler(msg, s, conn)

	}
}

func (s *Server) connectToPeer(addr string) (net.Conn, error) {
	s.Member.Lock()
	defer s.Member.Unlock()
	member := s.Member.GetMember(addr)
	if member != nil && member.GetClient() != nil {
		return member.GetClient(), nil
	}
	// 赋值给 Dialer 的 LocalAddr
	conn, err := s.client.Dial("tcp", addr)
	if err != nil {
		log.Printf("Failed to connect to %s: %v\n", addr, err)
		return nil, err
	}
	log.Printf("%sConnected to %s\n", s.Config.ServerAddress, addr)
	metaData := membership.NewEmptyMetaData()
	metaData.SetClient(conn)
	s.Member.PutMemberIfNil(addr, metaData)
	return conn, nil
}
func (s *Server) SendMessage(ip string, payload []byte, msg []byte) {

	if s.isClosed {
		return
	}
	metaData := s.Member.GetMember(ip)
	var conn net.Conn
	var err error
	if metaData == nil {
		conn, err = s.connectToPeer(ip)
		if err != nil {
			log.Println(s.Config.ServerAddress, "can't connect to ", ip)
			return
		}
	} else {
		conn = metaData.GetClient()
	}
	if conn == nil {
		//先建立一次链接进行尝试
		newConn, err := s.connectToPeer(ip)
		if err != nil {
			log.Println(s.Config.ServerAddress, "can't connect to ", ip)
			return
		} else {
			conn = newConn
		}
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
	s.sendChan <- data
}

// 这个方法只能用来回复消息
func (s *Server) replayMessage(conn net.Conn, config *Config, msg []byte) {
	// 创建消息头，存储消息长度 (4字节大端序)
	length := uint32(len(msg))
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, length)
	fmt.Println(conn.RemoteAddr().String())
	data := &SendData{
		Conn:    conn,
		Header:  header,
		Payload: []byte{},
		Msg:     msg,
	}
	s.sendChan <- data
}

// Close 关闭服务器
func (s *Server) Close() {
	s.listener.Close()
	s.Member.Lock()
	for _, v := range s.Member.MetaData {

		v.GetClient().Close()
		v.GetServer().Close()
	}

	s.Member.Unlock()
}

func (s *Server) Sender() {
	for data := range s.sendChan {
		_, err := data.Conn.Write(data.Header)
		if err != nil {
			s.ReportLeave(tool.IPv4To6Bytes(data.Conn.RemoteAddr().String()))
			log.Printf("Error sending header to %v: %v", data.Conn.RemoteAddr(), err)
			continue
		}
		_, err = data.Conn.Write(data.Payload)
		if err != nil {
			s.ReportLeave(tool.IPv4To6Bytes(data.Conn.RemoteAddr().String()))
			log.Printf("Error sending payload to %v: %v", data.Conn.RemoteAddr(), err)
			continue
		}
		_, err = data.Conn.Write(data.Msg)
		if err != nil {
			s.ReportLeave(tool.IPv4To6Bytes(data.Conn.RemoteAddr().String()))
			log.Printf("Error sending message to %v: %v", data.Conn.RemoteAddr(), err)
			continue
		}
		if s.Config.Test {
			bytes := append(data.Payload, data.Msg...)
			tool.SendHttp(s.Config.ServerAddress, data.Conn.RemoteAddr().String(), bytes, s.Config.FanOut)
		}
	}
}
