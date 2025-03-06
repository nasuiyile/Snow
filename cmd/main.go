package main

import (
	"fmt"
	"log"
	"snow/internal/broadcast"
	"time"
)

func main() {
	configPath := "E:\\code\\go\\Snow\\config\\config.yml"
	n := 10
	clientAddresses := initAddress(n)
	serverList := make([]*broadcast.Server, 0)
	for i := 0; i < n; i++ {
		server, err := broadcast.NewServer(5000+i, configPath, clientAddresses)
		if err != nil {
			return
		}
		serverList = append(serverList, server)
	}

	defer func() {
		for _, v := range serverList {
			v.Close()
		}
	}()

	// 模拟每隔1秒向所有客户端发送一条消息
	go func() {
		for {
			time.Sleep(3 * time.Second)
			err := serverList[5].BroadMessage("Hello from server!")
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
