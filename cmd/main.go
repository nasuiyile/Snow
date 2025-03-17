package main

import (
	"fmt"
	"log"
	"snow/internal/broadcast"
	"time"
)

func main() {
	configPath := "E:\\code\\go\\Snow\\config\\config.yml"
	n := 600
	initPort := 40000
	serverList := make([]*broadcast.Server, 0)
	//serversAddresses := initAddress(n, initPort)
	action := createAction()

	for i2 := 0; i2 < n; i2++ {
		f := func(config *broadcast.Config) {
			config.Port = initPort + i2
			config.DefaultServer = make([]string, 0)
		}
		config, err := broadcast.NewConfig(configPath, f)
		//time.Sleep(50 * time.Millisecond)
		server, err := broadcast.NewServer(config, action)
		if err != nil {
			return
		}
		serverList = append(serverList, server)
	}
	//模拟每隔1秒向所有客户端发送一条消息
	go func() {
		for i := 0; i < 50000000000000; i++ {
			time.Sleep(5 * time.Second)
			err := serverList[0].RegularMessage([]byte("hello from server!"), 0)
			if err != nil {
				log.Println("Error broadcasting message:", err)
			}
			//time.Sleep(2 * time.Second)
		}
	}()
	// 主线程保持运行
	select {}
}

func initAddress(n int, port int) []string {
	strings := make([]string, 0)
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port+i)
		strings = append(strings, addr)
	}
	return strings
}

func createAction() broadcast.Action {
	syncAction := func(bytes []byte) bool {
		s := string(bytes)

		fmt.Println("这里是同步处理消息的逻辑：", s)
		return true
	}
	asyncAction := func(bytes []byte) {
		//s := string(bytes)
		//fmt.Println("这里是异步处理消息的逻辑：", s)
	}
	reliableCallback := func(isConverged bool) {
		fmt.Println("这里是：可靠消息回调------------------------------", isConverged)
	}
	action := broadcast.Action{
		SyncAction:       &syncAction,
		AsyncAction:      &asyncAction,
		ReliableCallback: &reliableCallback,
	}
	return action
}
