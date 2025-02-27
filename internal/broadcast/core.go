package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// Server 定义服务器结构体
type Server struct {
	listener net.Listener
	clients  map[net.Conn]bool // 存储所有客户端连接
	mu       sync.Mutex        // 保护 clients 的并发访问
	Config   Config
}

// NewServer 创建并启动一个 TCP 服务器
func NewServer(port int, clientList []string) (*Server, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return nil, err
	}

	server := &Server{
		listener: listener,
		clients:  make(map[net.Conn]bool),
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

		s.mu.Lock()
		s.clients[conn] = true // 将新连接加入 clients
		s.mu.Unlock()

		go s.handleConnection(conn) // 处理客户端连接
	}
}

// handleConnection 处理客户端连接
func (s *Server) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.clients, conn) // 移除关闭的连接
		s.mu.Unlock()
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
		handler(msg)

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

	s.mu.Lock()
	s.clients[conn] = true // 将新连接加入 clients
	s.mu.Unlock()

	go s.handleConnection(conn) // 处理客户端连接
}

// SendMessage 向所有客户端发送消息
func (s *Server) SendMessage(message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.clients) == 0 {
		return fmt.Errorf("no clients connected")
	}

	// 将消息转换为字节数组
	messageBytes := []byte(message)

	// 创建消息头，存储消息长度 (4字节大端序)
	length := uint32(len(messageBytes))
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, length)

	// 遍历所有客户端并发送消息
	for conn := range s.clients {
		go func(c net.Conn) {
			_, err := c.Write(header)
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
	s.mu.Lock()
	for conn := range s.clients {
		conn.Close()
	}
	s.mu.Unlock()
}

func main() {
	_, err := NewServer(5000, []string{})

	// 将客户端列表字符串拆分为数组
	clientAddresses := []string{"localhost:5000"}

	// 创建服务器
	server, err := NewServer(5001, clientAddresses)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	// 模拟每隔5秒向所有客户端发送一条消息
	go func() {
		for {
			time.Sleep(5 * time.Second)
			err := server.SendMessage("Hello from server!")
			if err != nil {
				log.Println("Error broadcasting message:", err)
			}
		}
	}()

	// 主线程保持运行
	select {}
}
