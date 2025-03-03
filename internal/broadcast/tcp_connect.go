package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"
)

// NewServer 创建并启动一个 TCP 服务器
func NewServer(port int, clientList []string) (*Server, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, err
	}

	server := &Server{
		listener: listener,
		Config:   Config{Ipv6: false, IpAddress: fmt.Sprintf("%s%d", "127.0.0.1:", port), FanOut: 2},
		Member: MemberShipList{
			IPTable:  make([][]byte, 0),
			MetaData: make(map[string]MetaData),
		},
	}

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
	conn, err := net.Dial("tcp", addr)

	if err != nil {
		log.Printf("Failed to connect to %s: %v\n", addr, err)
		return
	}

	log.Printf("Connected to %s\n", addr)
	s.Member.AddNode(conn, true)
	go s.handleConnection(conn) // 处理客户端连接
}

// SendMessage 向对应
func (s *Server) SendMessage(message string) error {
	s.Member.lock.Lock()
	defer s.Member.lock.Unlock()

	if len(s.Member.MetaData) == 0 {
		fmt.Println("broadcasting message: no clients connected")
	}
	member := s.InitMessage()

	// 将消息转换为字节数组
	messageBytes := []byte(message)

	// 创建消息头，存储消息长度 (4字节大端序)
	length := uint32(len(messageBytes) + s.Config.Placeholder())
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, length)

	if len(member) == 0 {
		log.Printf("There are no other nodes in the member list")
	}
	for ip, payload := range member {
		conn := s.Member.MetaData[ip].clients
		if conn == nil {
			continue
		}
		go func(c net.Conn) {
			//写入消息包的大小
			_, err := c.Write(header)
			if err != nil {
				log.Printf("Error sending header to %v: %v", c.RemoteAddr(), err)
				return
			}
			_, err = c.Write(payload)
			if err != nil {
				log.Printf("Error sending header to %v: %v", c.RemoteAddr(), err)
				return
			}

			_, err = c.Write(messageBytes)
			if err != nil {
				log.Printf("Error sending body to %v: %v", c.RemoteAddr(), err)
				return
			}
		}(conn)
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

func main() {
	_, err := NewServer(5500, []string{})

	// 将客户端列表字符串拆分为数组
	clientAddresses := []string{"127.0.0.1:5500"}

	// 创建服务器
	server, err := NewServer(5001, clientAddresses)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	// 模拟每隔1秒向所有客户端发送一条消息
	go func() {
		for {
			time.Sleep(1 * time.Second)
			err := server.SendMessage("Hello from server!")
			if err != nil {
				log.Println("Error broadcasting message:", err)
			}
		}
	}()

	// 主线程保持运行
	select {}
}
