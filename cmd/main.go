package main

import (
	"fmt"
	"log"
	"snow/internal/broadcast"
	"time"
)

func main() {

	configPath := "E:\\code\\go\\Snow\\config\\config.yml"
	_, err := broadcast.NewServer(5000, configPath, []string{})
	// 将客户端列表字符串拆分为数组
	clientAddresses := initAddress(10)
	// 创建服务器
	server, err := broadcast.NewServer(5001, configPath, clientAddresses)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	server2, err := broadcast.NewServer(5002, configPath, clientAddresses)
	defer server.Close()

	// 模拟每隔1秒向所有客户端发送一条消息
	go func() {
		for {
			time.Sleep(5 * time.Second)
			err := server2.SendMessage("Hello from server!")
			if err != nil {
				log.Println("Error broadcasting message:", err)
			}
		}
	}()

	// 主线程保持运行
	select {}
}

func initAddress(n int) []string {
	strings := make([]string, 0)
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("127.0.0.1:%d", 5000+i)
		strings = append(strings, addr)
	}
	return strings
}
