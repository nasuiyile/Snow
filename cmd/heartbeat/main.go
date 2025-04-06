package main

import (
	"bytes"
	"fmt"
	"snow/common"
	"snow/config"
	"snow/internal/broadcast"
	"snow/internal/membership"
	"snow/tool"
	"time"
)

func main() {
	configPath := "config/config.yml"
	n := 5
	initPort := 40000
	serverList := make([]*broadcast.Server, 0)
	action := createAction()

	for i2 := 0; i2 < n; i2++ {
		f := func(config *config.Config) {
			config.Port = initPort + i2
			config.DefaultServer = make([]string, 0)
		}
		config, err := config.NewConfig(configPath, f)
		if err != nil {
			fmt.Printf("Failed to create config: %v\n", err)
			return
		}

		server, err := broadcast.NewServer(config, action)
		if err != nil {
			fmt.Printf("Failed to create server: %v\n", err)
			return
		}

		serverList = append(serverList, server)
		addNodesToIPTable(server, initPort)

		// 启动心跳服务
		if server.HeartbeatService != nil {
			server.HeartbeatService.Start()
			fmt.Printf("Started heartbeat service for server %d\n", i2)
		} else {
			fmt.Printf("Warning: heartbeat service not initialized for server %d\n", i2)
		}
	}

	// 添加一些测试逻辑，查看节点状态变化
	go func() {
		// 等待服务器初始化完成
		time.Sleep(3 * time.Second)

		// 每隔一段时间查看集群状态
		for {
			time.Sleep(5 * time.Second)
			for i, server := range serverList {
				fmt.Printf("Server %d status:\n", i)
				printMembershipStatus(server)
			}
		}
	}()

	select {}
}

func printMembershipStatus(server *broadcast.Server) {
	server.Member.Lock()
	defer server.Member.Unlock()

	fmt.Printf("  Total members: %d\n", len(server.Member.IPTable))
	for addr, meta := range server.Member.MetaData {
		stateStr := "Unknown"
		switch meta.State {
		case 0:
			stateStr = "NodePrepare"
		case 1:
			stateStr = "NodeSurvival"
		case 2:
			stateStr = "NodeSuspected"
		case 3:
			stateStr = "NodeLeft"
		}
		fmt.Printf("  %s: State=%s, LastUpdate=%v\n",
			addr, stateStr, time.Unix(meta.UpdateTime, 0))
	}
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
	server.Member.Lock()
	defer server.Member.Unlock()

	for i := 0; i < 5; i++ {
		port := initPort + i
		ip := "127.0.0.1" // 使用本地 IP 地址

		// 将 IP 和端口组合成目标地址，添加到 IPTable 中
		addr := fmt.Sprintf("%s:%d", ip, port)
		ipBytes := tool.IPv4To6Bytes(addr)

		// 判断是否已经存在
		exists := false
		for _, existing := range server.Member.IPTable {
			if bytes.Equal(existing, ipBytes) {
				exists = true
				break
			}
		}

		if !exists {
			server.Member.IPTable = append(server.Member.IPTable, ipBytes)

			// 同时更新元数据
			meta, ok := server.Member.MetaData[addr]
			if !ok {
				meta = membership.NewEmptyMetaData()
				meta.State = common.NodeSurvival
				meta.UpdateTime = time.Now().Unix()
				server.Member.MetaData[addr] = meta
			}
		}
	}
}
