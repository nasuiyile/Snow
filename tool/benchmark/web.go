package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/schema"
)

var cacheMap map[string]*MessageCache
var msgIdMap map[byte]map[string]int
var rm sync.RWMutex

func getMessageId(m Message) string {
	rm.Lock()
	defer rm.Unlock()

	if _, e := msgIdMap[m.MsgType]; !e {
		msgIdMap[m.MsgType] = make(map[string]int)
	}
	if _, e := msgIdMap[m.MsgType][m.Id]; !e {
		msgIdMap[m.MsgType][m.Id] = len(msgIdMap[m.MsgType]) + 1
	}
	num := msgIdMap[m.MsgType][m.Id]
	return fmt.Sprintf("%d", num)
}

// 接收节点广播的消息
func putRing(w http.ResponseWriter, r *http.Request) {
	decoder := schema.NewDecoder()
	message :=
		Message{}
	err := decoder.Decode(&message, r.URL.Query())
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Println(message)
	message.Timestamp = int(time.Now().UnixMilli())
	message.Id = getMessageId(message)

	if _, exisit := cacheMap[message.Id]; !exisit {
		cacheMap[message.Id] = CreateMessageCache()
	}
	cacheMap[message.Id].put(message)
}

// 消息传递路径
func getRing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	nodeArr := make([]Message, 0)
	for _, cache := range cacheMap {
		nodeArr = append(nodeArr, cache.getMessages()...)
	}

	var builder strings.Builder
	marshal, _ := json.Marshal(nodeArr)
	builder.WriteString(string(marshal))

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(builder.String()))
}

func getAllMessage(w http.ResponseWriter, r *http.Request) {
	messageMap := make(map[string][]Message)

	for k, v := range cacheMap {
		if _, exisit := messageMap[k]; !exisit {
			messageMap[k] = make([]Message, 0)
		}
		messageMap[k] = append(messageMap[k], v.getMessages()...)
	}

	var builder strings.Builder
	marshal, _ := json.Marshal(messageMap)
	builder.WriteString(string(marshal))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(builder.String()))
}

func getAllNode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	nodeMap := make(map[string][]*MessageNode, 0)

	for k, v := range cacheMap {
		if _, exisit := nodeMap[k]; !exisit {
			nodeMap[k] = make([]*MessageNode, 0)
		}
		nodeMap[k] = append(nodeMap[k], v.getNodes()...)
	}

	var builder strings.Builder
	marshal, _ := json.Marshal(nodeMap)
	builder.WriteString(string(marshal))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(builder.String()))
}

func totalCount(w http.ResponseWriter, r *http.Request) {

}

func clean(w http.ResponseWriter, r *http.Request) {
	cacheMap = make(map[string]*MessageCache)
	msgIdMap = make(map[byte]map[string]int)
}

func getNodeStatistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 获取全部轮次每个节点的消息量
	nodeArr := make([]*MessageNode, 0)
	for _, v := range cacheMap {
		nodeArr = append(nodeArr, v.getNodes()...)
	}

	nodeMap := make(map[string][]MessageNode, 0)
	// 将全部轮次的节点按节点分组
	for _, node := range nodeArr {
		if _, e := nodeMap[node.Node]; !e {
			nodeMap[node.Node] = make([]MessageNode, 0)
		}
		nodeMap[node.Node] = append(nodeMap[node.Node], *node)
	}

	// 统计每个节点在多个轮次下的信息
	nodeStatistic := make(map[string]MessageNode)
	for k, v := range nodeMap {
		fanInCount := 0
		fanOutCount := 0
		flowInSum := 0
		flowOutSum := 0
		flowInAvg := 0.0
		flowOutAvg := 0.0
		for _, node := range v {
			fanInCount += node.FanIn
			fanOutCount += node.FanOut
			flowInSum += node.FlowIn
			flowOutSum += node.FlowOut
		}
		flowInAvg = float64(flowInSum) / float64(len(v))
		flowOutAvg = float64(flowOutSum) / float64(len(v))
		flowInS := 0.0
		flowOutS := 0.0
		for _, node := range v {
			flowInS += math.Pow(float64(node.FlowIn)-flowInAvg, 2)
			flowOutS += math.Pow(float64(node.FlowOut)-flowOutAvg, 2)
		}
		flowInS = flowInS / float64(len(v))
		flowOutS = flowOutS / float64(len(v))
		nodeStatistic[k] = MessageNode{
			Node:  k,
			FanIn: fanInCount, FanOut: fanOutCount,
			FlowIn: flowInSum, FlowOut: flowOutSum,
			FlowInAvg: flowInAvg, FlowOutAvg: flowOutAvg,
			FlowInS: flowInS, FlowOutS: flowOutS,
		}
	}

	var builder strings.Builder
	marshal, _ := json.Marshal(nodeStatistic)
	builder.WriteString(string(marshal))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(builder.String()))
}

func getCycleStatistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 统计每个轮次的消息信息
	cycleTypeMap := make(map[byte]map[string]MessageCycle)
	cycleMap := make(map[string]MessageCycle)
	for msgType, _ := range msgIdMap {
		for k, v := range cacheMap {
			messageGroup := v.getMessagesByGroup(msgType)
			nodeCount := len(v.getMessages())
			cycle := staticticsCycle(messageGroup, nodeCount)
			cycleMap[k] = cycle
		}
		cycleTypeMap[msgType] = cycleMap
	}

	var builder strings.Builder
	marshal, _ := json.Marshal(cycleTypeMap)
	builder.WriteString(string(marshal))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(builder.String()))
}
func CreateWeb() {
	cacheMap = make(map[string]*MessageCache)
	msgIdMap = make(map[byte]map[string]int)
	// 注册路由和处理函数
	http.HandleFunc("/putRing", putRing)
	http.HandleFunc("/getRing", getRing)
	http.HandleFunc("/totalCount", totalCount)
	http.HandleFunc("/clean", clean)
	http.HandleFunc("/getAllMessage", getAllMessage)
	http.HandleFunc("/getAllNode", getAllNode)
	http.HandleFunc("/getNodeStatistics", getNodeStatistics)
	http.HandleFunc("/getCycleStatistics", getCycleStatistics)

	// 启动HTTP服务器
	fmt.Println("Server is running on http://localhost:8111")
	if err := http.ListenAndServe(":8111", nil); err != nil {
		fmt.Printf("Error starting server: %s\n", err)
	}
}

func main() {
	CreateWeb()
}
