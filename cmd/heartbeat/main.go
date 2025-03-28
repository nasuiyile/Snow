package main

import (
	"fmt"
	"log"
	"snow/internal/broadcast"
	"snow/internal/broadcast/heartbeat"
	"snow/tool"
	"time"
)

func main() {
	initPort := 40000
	action := createAction()

	// 构造测试节点列表（包括5个节点）
	var nodes [][6]byte

	// 启动多个服务器，每个服务器通过 broadcast.NewServer 启动 TCP & UDP 服务
	for i := 0; i < 5; i++ {
		f := func(cfg *broadcast.Config) {
			cfg.Port = initPort + i
			cfg.DefaultServer = []string{}
		}
		config, err := broadcast.NewConfig("./config/config.yml", f)
		if err != nil {
			log.Fatalf("NewConfig error: %v", err)
		}
		// NewServer 内部会启动 TCP 监听器，并处理连接回复 Ping 的 ack
		_, err = broadcast.NewServer(config, action)
		if err != nil {
			log.Fatalf("NewServer error: %v", err)
		}
		addrStr := fmt.Sprintf("127.0.0.1:%d", config.Port)
		b := tool.IPv4To6Bytes(addrStr)
		var node [6]byte
		copy(node[:], b)
		nodes = append(nodes, node)
	}

	// 等待几秒钟，确保所有服务器的 TCP 监听器均已启动
	time.Sleep(3 * time.Second)

	// 使用第一个服务器的配置创建 heartbeat 实例
	config, err := broadcast.NewConfig("./config/config.yml", func(cfg *broadcast.Config) {
		cfg.Port = initPort // 这里使用第一个服务器的端口
		cfg.DefaultServer = []string{}
	})
	if err != nil {
		log.Fatalf("NewConfig error: %v", err)
	}
	hb := heartbeat.NewHeartbeat(config)
	hb.SetIPTable(nodes)

	// 注册 suspect 回调函数
	hb.SetSuspectCallback(func(s heartbeat.Suspect) {
		log.Printf("[Heartbeat Callback] Node %s is suspected (incarnation %d) by %s",
			tool.ByteToIPv4Port(s.Addr[:]), s.Incarnation, tool.ByteToIPv4Port(s.Src[:]))
	})

	// 启动 heartbeat 探测
	hb.Start()

	log.Println("Heartbeat testing started, press Ctrl+C to exit.")
	select {}
}

func createAction() broadcast.Action {
	syncAction := func(b []byte) bool {
		s := string(b)
		fmt.Println("同步处理消息：", s)
		return true
	}
	asyncAction := func(b []byte) {
		// 异步处理逻辑（可选）
	}
	reliableCallback := func(isConverged bool) {
		fmt.Println("可靠消息回调：", isConverged)
	}
	return broadcast.Action{
		SyncAction:       &syncAction,
		AsyncAction:      &asyncAction,
		ReliableCallback: &reliableCallback,
	}
}
