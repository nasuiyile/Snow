package benchmark

import (
	"fmt"
	"log"
	"snow/internal/broadcast"
	"testing"
	"time"
)

func Test_boardcast(t *testing.T) {
	configPath := "..\\..\\config\\config.yml"

	// 节点数量
	n := 10
	initPort := 50000
	serverList := make([]*broadcast.Server, 0)
	//serversAddresses := initAddress(n)
	action := createAction()

	for i := 0; i < n; i++ {
		f := func(config *broadcast.Config) {
			config.Port = initPort + i
		}
		config, err := broadcast.NewConfig(configPath, f)
		if err != nil {
			return
		}
		server, err := broadcast.NewServer(config, action)
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

	// 测试轮数
	for i := range 5 {
		err := serverList[5].RegularMessage([]byte("hello from server!"), 0)
		if err != nil {
			log.Println("Error broadcasting message:", err)
		}
		// 1秒一轮
		time.Sleep(1 * time.Second)
		fmt.Printf("=== %d =====\n", i)
	}

	time.Sleep(10 * time.Second)

	// 主线程保持运行
	select {}
}

func createAction() broadcast.Action {
	syncAction := func(bytes []byte) bool {
		s := string(bytes)
		fmt.Println("这里是同步处理消息的逻辑：", s)
		return true
	}
	asyncAction := func(bytes []byte) {
		s := string(bytes)
		fmt.Println("这里是异步处理消息的逻辑：", s)
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
