package plumtree

import (
	"fmt"
	"log"
	"net/http"
	"snow/common"
	"snow/config"
	"snow/internal/broadcast"
	"snow/tool"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var lock sync.Mutex
var serverMap map[int]*Server
var portMap map[int]int

func TestPlumServer(t *testing.T) {
	serverMap = make(map[int]*Server)
	portMap = make(map[int]int)

	createServer()
	defer closeServer()
	startServer()

	for {
		time.Sleep(1 * time.Second)
	}
}

func closeServer() {
	for _, v := range serverMap {
		go func() {
			v.Close()
		}()
	}
	time.Sleep(3 * time.Second)
}

func createServer() {
	configPath := "..\\..\\config\\config.yml"
	n := 5
	k := 2
	initPort := 40000
	serversAddresses := initAddress(n, initPort)
	for i := range n {
		action := createAction(i + 1)
		f := func(config *config.Config) {
			config.Port = initPort + i
			config.FanOut = k
			config.DefaultServer = serversAddresses
		}
		config, err := config.NewConfig(configPath, f)
		if err != nil {
			panic(err)
		}
		server, err := NewServer(config, action)
		if err != nil {
			log.Println(err)
			return
		}

		serverMap[initPort+i] = server
	}
}

func startServer() {
	http.HandleFunc("/applyLeave", applyLeave)
	http.HandleFunc("/plumBroadcast", plumBroadcast)

	for k, _ := range serverMap {
		go func() {
			lock.Lock()
			port := k + 100
			portMap[port] = k
			lock.Unlock()

			addr := fmt.Sprintf("localhost:%d", port)
			log.Println("http start " + addr)
			if err := http.ListenAndServe(addr, nil); err != nil {
				log.Printf("Error starting server: %s\n", err)
			}
		}()
	}
}

func applyLeave(w http.ResponseWriter, r *http.Request) {
	port := r.URL.Port()
	p, err := strconv.Atoi(port)
	if err != nil {
		log.Println("applyLeave err", err)
	}
	server := serverMap[p]
	server.ApplyLeave()
}

func plumBroadcast(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	log.Println(host, "plumBroadcast")

	port, err := strconv.Atoi(strings.Split(host, ":")[1])
	if err != nil {
		log.Println("Atoi err", err)
	}

	msg := "hello world"
	server := serverMap[portMap[port]]
	server.PlumTreeBroadcast([]byte(msg), common.UserMsg)
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

func initAddress(n int, port int) []string {
	strings := make([]string, 0)
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port+i)
		strings = append(strings, addr)
	}
	return strings
}
