package main

import (
	"fmt"
	"snow/internal/broadcast"
	"snow/internal/broadcast/heartbeat"
	"snow/tool"
)

func main() {
	configPath := "config/config.yml"
	n := 10
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
		serverList = append(serverList, server)
		addNodesToIPTable(server, initPort)
		heartbeat.NewHeartbeat(server)
		if err != nil {
			return
		}
	}

	select {}
}

func createAction() broadcast.Action {
	syncAction := func(bytes []byte) bool {
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

// 将节点手动添加到 IPTable 中
func addNodesToIPTable(server *broadcast.Server, initPort int) {
	for i := 0; i < 10; i++ {
		port := initPort + i
		ip := "127.0.0.1" // 使用本地 IP 地址

		// 将 IP 和端口组合成目标地址，添加到 IPTable 中
		addr := fmt.Sprintf("%s:%d", ip, port)
		server.Member.IPTable = append(server.Member.IPTable, tool.IPv4To6Bytes(addr))
	}
}
