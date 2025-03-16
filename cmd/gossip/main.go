package main

import (
	"fmt"
	"net"
	"syscall"
	"time"
)

func handleConnection(conn net.Conn) {
	defer conn.Close()
	fmt.Println("新客户端连接:", conn.RemoteAddr())
}

func main() {
	go s(":9090")
	go s(":9071")
	time.Sleep(3 * time.Second) // 等待服务器启动
	go main2()

	// 防止主函数退出
	select {}
}

func main2() {
	// 指定本地地址
	localAddr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:9000")
	if err != nil {
		fmt.Println("解析本地地址失败:", err)
		return
	}

	// 创建 Dialer，设置 Control 函数以允许端口复用
	dialer := net.Dialer{
		LocalAddr: localAddr,
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				//设置 SO_REUSEADDR
				err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				if err != nil {
					fmt.Println("设置 SO_REUSEADDR 失败:", err)
				}

			})
		},
	}
	// 连接到第二个服务器
	conn2, err := dialer.Dial("tcp", "127.0.0.1:9071")
	tcpConn := conn2.(*net.TCPConn)
	tcpConn.SetLinger(0)
	if err != nil {
		fmt.Println("连接失败:", err)
		return
	}
	fmt.Println("连接成功:", conn2.LocalAddr())
	conn2.Close()

	// 增加延迟，确保端口资源释放
	time.Sleep(1 * time.Second)

	// 断开之后重连
	conn2, err = dialer.Dial("tcp", "127.0.0.1:9071")
	tcpConn = conn2.(*net.TCPConn)
	tcpConn.SetLinger(0)
	if err != nil {
		fmt.Println("连接失败:", err)
		return
	}
	fmt.Println("连接成功:", conn2.LocalAddr())
	defer conn2.Close()
}

func s(port string) {
	listener, err := net.Listen("tcp", port)
	if err != nil {
		fmt.Println("监听失败:", err)
		return
	}
	defer listener.Close()
	fmt.Println("服务器启动，监听端口", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("接受连接失败:", err)
			continue
		}
		go handleConnection(conn) // 使用 goroutine 处理每个客户端
	}
}
