package broadcast

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"snow/tool"
)

// NewServer 创建并启动一个 TCP 服务器
func NewServer(port int, configPath string, clientList []string) (*Server, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		panic(err)
	}
	config.LocalAddress = fmt.Sprintf("%s:%d", config.LocalAddress, port)
	listener, err := net.Listen("tcp", config.LocalAddress)
	if err != nil {
		return nil, err
	}
	server := &Server{
		listener: listener,
		Config:   config,
		Member: MemberShipList{
			IPTable:  make([][]byte, 0),
			MetaData: make(map[string]*MetaData),
		},
	}
	server.Member.FindOrInsert(IPv4To6Bytes(config.LocalAddress))
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
		conn, err := s.listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		s.Member.AddNode(conn, false)
		go s.handleConnection(conn) // 处理客户端连接
	}
}

// handleConnection 处理客户端连接
func (s *Server) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		s.Member.lock.Lock()
		delete(s.Member.MetaData, conn.RemoteAddr().String()) // 移除关闭的连接
		s.Member.lock.Unlock()
	}()

	reader := bufio.NewReader(conn)
	for {
		// 读取消息头 (4字节表示消息长度)
		header := make([]byte, 4)
		_, err := reader.Read(header)
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
		_, err = reader.Read(msg)
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
func (s *Server) connectToClient(addr string) {
	conn, err := s.connectToPeer(addr)
	if err != nil {
		return
	}
	go s.handleConnection(conn) // 处理客户端连接
}
func (s *Server) connectToPeer(addr string) (net.Conn, error) {
	if s.Config.Test {
		s.Member.lock.Lock()
		defer s.Member.lock.Unlock()
		s.Member.MetaData[addr] = NewMetaData(nil)
		s.Member.FindOrInsert(IPv4To6Bytes(addr))
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Printf("Failed to connect to %s: %v\n", addr, err)
		return nil, err
	}
	log.Printf("Connected to %s\n", addr)
	s.Member.AddNode(conn, true)
	return conn, nil
}

// SendMessage 向对应
func (s *Server) SendMessage(message string) error {
	s.Member.lock.Lock()
	defer s.Member.lock.Unlock()

	if len(s.Member.MetaData) == 0 {
		log.Println("broadcasting message: no clients connected")
	}

	// 将消息转换为字节数组
	messageBytes := []byte(message)

	// 创建消息头，存储消息长度 (4字节大端序)
	length := uint32(len(messageBytes) + s.Config.Placeholder())
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, length)
	member := s.InitMessage(userMsg)
	if len(member) == 0 {
		log.Printf("There are no other nodes in the member list")
	}
	for ip, payload := range member {
		conn := s.Member.MetaData[ip].clients
		if conn == nil {
			//先建立一次链接进行尝试
			newConn, err := s.connectToPeer(ip)
			if err != nil {
				log.Println("can't connect to ", ip)
				continue
			} else {
				s.Member.MetaData[ip].clients = newConn
			}
		}
		go func(c net.Conn, s *Server) {

			//写入消息包的大小
			_, err := c.Write(header)
			if err != nil {
				log.Printf("Error sending header to %v: %v", c.RemoteAddr(), err)
				return
			}
			//因为tcp会帮你填0所以一定要一起发送
			allMessage := append(payload, messageBytes...)
			_, err = c.Write(allMessage)
			if err != nil {
				log.Printf("Error sending header to %v: %v", c.RemoteAddr(), err)
				return
			}
			if s.Config.Test {
				tool.SendHttp(s.Config.LocalAddress, ip, allMessage)
			}
		}(conn, s)
	}

	return nil
}

// ForwardMessage SendMessage 向对应
func (s *Server) ForwardMessage(msg []byte, member map[string][]byte) error {
	for ip, payload := range member {
		conn := s.Member.MetaData[ip].clients
		if conn == nil {
			//先建立一次链接进行尝试
			newConn, err := s.connectToPeer(ip)
			if err != nil {
				log.Println(s.Config.LocalAddress, "can't connect to ", ip)
				continue
			} else {
				s.Member.MetaData[ip].clients = newConn
			}
		}
		// 创建消息头，存储消息长度 (4字节大端序)
		length := uint32(len(msg))
		header := make([]byte, 4)
		binary.BigEndian.PutUint32(header, length)

		if len(member) == 0 {
			log.Printf("can't forward! membership is empty")
		}
		go func(c net.Conn, s *Server) {
			//写入消息包的大小
			_, err := c.Write(header)
			if err != nil {
				log.Printf("Error sending header to %v: %v", c.RemoteAddr(), err)
				return
			}
			//因为tcp会帮你填0所以一定要一起发送
			copy(msg, payload)
			_, err = c.Write(msg)
			if err != nil {
				log.Printf("Error sending header to %v: %v", c.RemoteAddr(), err)
				return
			}
		}(conn, s)
	}
	return nil
}

// Close 关闭服务器
func (s *Server) Close() {
	s.listener.Close()
	s.Member.lock.Lock()
	for _, v := range s.Member.MetaData {
		v.clients.Close()
	}

	s.Member.lock.Unlock()
}
