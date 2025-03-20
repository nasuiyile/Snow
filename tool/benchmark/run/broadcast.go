package main

import (
	"fmt"
	"log"
	"math/rand"
	"snow/common"
	"snow/internal/broadcast"
	"snow/tool"
	"time"
)

func main() {
	configPath := "E:\\code\\go\\Snow\\config\\config.yml"

	// 节点数量
	n := 500
	//扇出大小
	k := 2

	//消息大小
	strLen := 100
	//测试轮数
	rounds := 40
	initPort := 40000

	serversAddresses := initAddress(n, initPort)

	msg := randomByteArray(strLen)
	serverList := make([]*broadcast.Server, 0)
	//serversAddresses := initAddress(n)
	for i := 0; i < n; i++ {
		action := createAction(i + 1)
		f := func(config *broadcast.Config) {
			config.Port = initPort + i
			config.FanOut = k
			config.DefaultServer = serversAddresses
		}
		config, err := broadcast.NewConfig(configPath, f)
		if err != nil {
			panic(err)
			return
		}
		server, err := broadcast.NewServer(config, action)
		if err != nil {
			log.Println(err)
			return
		}
		serverList = append(serverList, server)
	}

	defer func() {
		for _, v := range serverList {
			v.Close()
		}
	}()
	//节点启动完之后再跑
	time.Sleep(time.Duration(n/100) * time.Second)
	for i := range rounds {
		// 1秒一轮,节点可能还没有离开新的广播就发出了	4秒足够把消息广播到所有节点
		fmt.Printf("=== %d =====\n", i)
		time.Sleep(1 * time.Second)
		err := serverList[0].ColoringMessage(msg, common.UserMsg)
		//time.Sleep(4 * time.Second)
		//var removedNode *broadcast.Server
		//if len(serverList) > 1 {
		//	removedNode, serverList = tool.RemoveElement(serverList, 1)
		//}
		//removedNode.ApplyLeave()
		//tool.Num--
		if err != nil {
			log.Println("Error broadcasting message:", err)
		}
	}
	//// 测试轮数
	for i := range rounds {
		// 1秒一轮,节点可能还没有离开新的广播就发出了	4秒足够把消息广播到所有节点
		fmt.Printf("=== %d =====\n", i)
		time.Sleep(1 * time.Second)
		err := serverList[0].RegularMessage(msg, common.UserMsg)
		//time.Sleep(4 * time.Second)
		//var removedNode *broadcast.Server
		//if len(serverList) > 1 {
		//	removedNode, serverList = tool.RemoveElement(serverList, 1)
		//}
		//removedNode.ApplyLeave()
		//tool.Num--
		if err != nil {
			log.Println("Error broadcasting message:", err)
		}
	}

	for i := range rounds {
		// 1秒一轮,节点可能还没有离开新的广播就发出了	4秒足够把消息广播到所有节点
		fmt.Printf("=== %d =====\n", i)
		time.Sleep(1 * time.Second)
		err := serverList[0].GossipMessage(msg, common.UserMsg)
		//time.Sleep(4 * time.Second)
		//var removedNode *broadcast.Server
		//if len(serverList) > 1 {
		//	removedNode, serverList = tool.RemoveElement(serverList, 1)
		//}
		//removedNode.ApplyLeave()
		//tool.Num--
		if err != nil {
			log.Println("Error broadcasting message:", err)
		}
	}

	time.Sleep(10 * time.Second)

	// 主线程保持运行
	select {}
}

// 编号从0开始
func createAction(num int) broadcast.Action {
	syncAction := func(bytes []byte) bool {
		s := string(bytes)
		//随机睡眠时间，百分之5的节点是掉队者节点
		if num%20 == 0 {
			time.Sleep(1 * time.Second)
		} else {
			randInt := tool.RandInt(10, 200)
			//randInt := tool.RandInt(100, 200)

			time.Sleep(time.Duration(randInt) * time.Millisecond)
		}

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
func randomByteArray(length int) []byte {
	rand.Seed(time.Now().UnixNano()) // 设置随机种子
	bytes := make([]byte, length)

	for i := range bytes {
		bytes[i] = byte(rand.Intn(26) + 'a') // 生成 a-z 之间的随机字符
	}

	return bytes
}
func initAddress(n int, port int) []string {
	strings := make([]string, 0)
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port+i)
		strings = append(strings, addr)
	}
	return strings
}
