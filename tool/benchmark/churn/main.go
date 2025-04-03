package main

import (
	"fmt"
	"log"
	"math/rand"
	. "snow/common"
	"snow/internal/broadcast"
	"snow/internal/plumtree"
	"snow/tool"
	"time"
)

func main() {

	////测试轮数
	rounds := 100
	benchmark(500, 4, rounds)
	fmt.Println("done!!!")
	// 主线程保持运行
	select {}
}
func benchmark(n int, k int, rounds int) {
	configPath := "E:\\code\\go\\Snow\\config\\config.yml"
	//消息大小
	strLen := 100
	initPort := 40000
	testMode := []MsgType{EagerPush, RegularMsg, ColoringMsg, GossipMsg} //按数组中的顺序决定跑的时候的顺序
	serversAddresses := initAddress(n, initPort)
	tool.Num = n
	tool.InitPort = initPort
	msg := randomByteArray(strLen)
	//节点启动完之后再跑
	time.Sleep(time.Duration(n/200) * time.Second)
	portCounter := 0

	for _, mode := range testMode {
		serverList := make([]*plumtree.Server, 0)
		serverTempList := make([]*plumtree.Server, 0)

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
			server, err := plumtree.NewServer(config, action)
			if err != nil {
				log.Println(err)
				return
			}
			serverList = append(serverList, server)
		}
		//每次发r个消息加入一个节点
		r := 10
		for i := 0; i < rounds; i = i + r {
			action := createAction(1)
			f := func(config *broadcast.Config) {
				portCounter++
				config.Port = initPort + n + 1 + portCounter
				config.FanOut = k
				config.DefaultServer = serversAddresses
				config.Report = true
			}
			config, err := broadcast.NewConfig(configPath, f)
			if err != nil {
				panic(err)
				return
			}
			server, err := plumtree.NewServer(config, action)
			if err != nil {
				log.Println(err)
				return
			}
			serverTempList = append(serverTempList, server)
			if mode == EagerPush {
				server.ApplyJoin()
			} else {
				server.Server.ApplyJoin(server.Config.InitialServer)
			}

			for i2 := 0; i2 < r; i2++ {
				// 1秒一轮,节点可能还没有离开新的广播就发出了	4秒足够把消息广播到所有节点
				fmt.Printf("=== %d =====\n", i+i2)
				time.Sleep(1000 * time.Millisecond)
				if mode == RegularMsg {
					serverList[0].RegularMessage(msg, UserMsg)
				} else if mode == ColoringMsg {
					serverList[0].ColoringMessage(msg, UserMsg)
				} else if mode == GossipMsg {
					serverList[0].GossipMessage(msg, UserMsg)
				} else if mode == EagerPush {
					serverList[0].PlumTreeBroadcast(msg, UserMsg)
				}
			}

			if mode == EagerPush {
				server.ApplyLeave()
			} else {
				server.Server.ApplyLeave()
			}
		}
		if mode == EagerPush {
			time.Sleep(35 * time.Second)
		} else {
			time.Sleep(5 * time.Second)
		}
		//释放所有链接重新跑
		for _, v := range serverList {
			v.Close()
		}
		for _, v := range serverTempList {
			v.Close()
		}
	}

}

// 编号从0开始
func createAction(num int) broadcast.Action {
	syncAction := func(bytes []byte) bool {
		//随机睡眠时间，百分之5的节点是掉队者节点
		if num%20 == 0 {
			time.Sleep(1 * time.Second)
		} else {
			randInt := tool.RandInt(10, 200)
			time.Sleep(time.Duration(randInt) * time.Millisecond)
		}

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
