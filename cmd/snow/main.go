package main

import (
	"fmt"
	"log"
	"snow/config"
	"snow/internal/broadcast"
	"snow/pkg/server"
	"time"
)

func main() {
	configPath := "E:\\code\\go\\Snow\\config\\config.yml"
	n := 20
	initPort := 20000
	serverList := make([]*server.SnowServer, 0)
	serversAddresses := initAddress(n, initPort)
	action := createAction()

	for i2 := 0; i2 < n; i2++ {
		f := func(config *config.Config) {
			config.Port = initPort + i2
			config.DefaultServer = serversAddresses
		}
		config, err := config.NewConfig(configPath, f)
		//time.Sleep(50 * time.Millisecond)
		server, err := server.NewSnowServer(config, action)
		if err != nil {
			return
		}
		serverList = append(serverList, server)
	}
	//模拟每隔1秒向所有客户端发送一条消息
	go func() {
		for i := 0; i < 50000000000000; i++ {
			time.Sleep(1 * time.Second)
			serverList[0].StandardMessage([]byte("hello from server!"))
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
		log.Println("这里是同步处理消息的逻辑：", s)
		return true
	}
	asyncAction := func(bytes []byte) {
		//s := string(bytes)
		//fmt.Println("这里是异步处理消息的逻辑：", s)
	}
	reliableCallback := func(isConverged bool) {
		log.Println("这里是：可靠消息回调------------------------------", isConverged)
	}
	action := broadcast.Action{
		SyncAction:       &syncAction,
		AsyncAction:      &asyncAction,
		ReliableCallback: &reliableCallback,
	}
	return action
}
