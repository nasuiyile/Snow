package broadcast

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"snow/internal/membership"
	"snow/internal/state"
	"snow/tool"
	"syscall"
)

// NewServer 创建并启动一个 TCP 服务器
func NewServer(port int, configPath string, clientList []string, action Action) (*Server, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		panic(err)
	}
	config.ClientAddress = fmt.Sprintf("%s:%d", config.LocalAddress, port+config.ClientPortOffset)
	config.LocalAddress = fmt.Sprintf("%s:%d", config.LocalAddress, port)
	listener, err := net.Listen("tcp", config.LocalAddress)
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
		Action: action,
		client: net.Dialer{
			LocalAddr: clientAddress,
			Control: func(network, address string, c syscall.RawConn) error {
				return c.Control(func(fd uintptr) {
					// 设置 SO_REUSEADDR
					err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
					if err != nil {
						fmt.Println("设置 SO_REUSEADDR 失败:", err)
					}
				})
			},
		},
	}
	server.Member.FindOrInsert(tool.IPv4To6Bytes(config.LocalAddress))
	go server.startAcceptingConnections() // 启动接受连接的协程
	// 主动连接到其他客户端
	for _, addr := range clientList {
		go server.connectToClient(addr)
	}

	log.Printf("Server is running on port %d...\n\n", port)
	return server, nil
}

// startAcceptingConnections 不断接受新的客户端连接
func (s *Server) startAcceptingConnections() {
	for {
		//其实是由netpoller 来触发的
		conn, err := s.listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		serverIp := s.Config.GetServerIp(conn.RemoteAddr().String())
		metaData := membership.NewEmptyMetaData()
		metaData.SetServer(conn)
		s.Member.PutMemberIfNil(serverIp, metaData)
		// todo 后期优化成只能保持k个连接
		//s.Member.AddNode(conn, false)
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
		// 读取消息头 (4字节表示消息长度)
		header := make([]byte, 4)
		_, err := io.ReadFull(reader, header)
		if err != nil {
			log.Printf("Read header error from %v: %v\n", conn.RemoteAddr(), err)
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

		// 打印接收到的消息
		//fmt.Printf("Received message from %v: %s\n", conn.RemoteAddr(), string(body))

		//// 回复客户端
		//response := "Message received"
		//responseBytes := []byte(response)
		//
		//length := uint32(len(responseBytes))
		//binary.BigEndian.PutUint32(header, length)
		//
		//conn.Write(header)
		//conn.Write(responseBytes)
	}
}

// connectToClient 主动连接到其他客户端
func (s *Server) connectToClient(addr string) error {
	conn, err := s.connectToPeer(addr)
	if err != nil {
		log.Println("Error connection:", err)
		return err
	}
	go s.handleConnection(conn, true) // 处理客户端连接
	return nil
}
func (s *Server) connectToPeer(addr string) (net.Conn, error) {
	if s.Config.Test {
		s.Member.Lock()
		defer s.Member.Unlock()
		s.Member.PutMemberIfNil(addr, membership.NewEmptyMetaData())
		s.Member.FindOrInsert(tool.IPv4To6Bytes(addr))
	}

	// 赋值给 Dialer 的 LocalAddr
	conn, err := s.client.Dial("tcp", addr)
	if err != nil {
		log.Printf("Failed to connect to %s: %v\n", addr, err)
		return nil, err
	}
	log.Printf("Connected to %s\n", addr)
	s.Member.AddNode(conn, true)
	return conn, nil
}
func (s *Server) SendMessage(ip string, msg []byte) {
	metaData := s.Member.GetMember(ip)
	var conn net.Conn
	var err error
	if metaData == nil {
		//建立临时连接
		conn, err = s.client.Dial("tcp", ip)
		if err != nil {
			log.Printf("Failed to connect to %s: %v\n", ip, err)
			return
		}
	} else {
		conn = metaData.GetClient()
	}
	if conn == nil {
		//先建立一次链接进行尝试
		newConn, err := s.connectToPeer(ip)
		if err != nil {
			log.Println(s.Config.LocalAddress, "can't connect to ", ip)
			return
		} else {
			metaData = membership.NewEmptyMetaData()
			metaData.SetServer(conn)
			s.Member.PutMemberIfNil(ip, metaData)
			s.Member.MetaData[ip].SetClient(newConn)
			conn = newConn
		}
	}
	// 创建消息头，存储消息长度 (4字节大端序)
	length := uint32(len(msg))

	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, length)

	go func(c net.Conn, config *Config) {
		//写入消息包的大小
		_, err := c.Write(header)
		if err != nil {
			log.Printf("Error sending header to %v: %v", c.RemoteAddr(), err)
			return
		}
		_, err = c.Write(msg)
		if err != nil {
			log.Printf("Error sending header to %v: %v", c.RemoteAddr(), err)
			return
		}
		if s.Config.Test {
			tool.SendHttp(s.Config.LocalAddress, ip, msg)
		}
	}(conn, s.Config)
}

// 这个方法只能用来回复消息
func replayMessage(conn net.Conn, config *Config, msg []byte) {
	// 创建消息头，存储消息长度 (4字节大端序)
	length := uint32(len(msg))
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, length)
	go func(c net.Conn, config *Config) {
		//写入消息包的大小
		_, err := c.Write(header)
		if err != nil {
			log.Printf("Error sending header to %v: %v", c.RemoteAddr(), err)
			return
		}
		_, err = c.Write(msg)
		if err != nil {
			log.Printf("Error sending header to %v: %v", c.RemoteAddr(), err)
			return
		}
		if config.Test {
			tool.SendHttp(config.LocalAddress, conn.RemoteAddr().String(), msg)
		}
	}(conn, config)
}

// Close 关闭服务器
func (s *Server) Close() {
	s.listener.Close()
	s.Member.Lock()
	for _, v := range s.Member.MetaData {
		v.GetClient().Close()
	}

	s.Member.Unlock()
}
