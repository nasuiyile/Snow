package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	. "snow/common"
	"snow/tool"
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
	message := Message{}
	err := decoder.Decode(&message, r.URL.Query())
	if err != nil {
		log.Println(err)
		return
	}
	// 类型15会生成一堆id，暂时排除影响
	if message.MsgType == 15 {
		return
	}
	//fmt.Println(message)
	message.Timestamp = int(time.Now().UnixMilli())
	message.Id = getMessageId(message)
	rm.Lock()
	if _, exisit := cacheMap[message.Id]; !exisit {
		cacheMap[message.Id] = CreateMessageCache()
	}
	cacheMap[message.Id].put(message)
	rm.Unlock()
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
func lack(w http.ResponseWriter, r *http.Request) {
	rm.RLock()
	arr := make([][]string, 0)
	for _, v := range cacheMap {
		flag := false
		strArr := make([]string, 0)
		for i := 1; i < 300; i++ {
			strArr = append(strArr, fmt.Sprintf("%s%d", "127.0.0.1:", i+40000))
		}
		for _, msg := range v.messages {
			if msg.MsgType == RegularMsg {
				flag = true
			}
			strArr = tool.RemoveElement(strArr, msg.Target)
		}
		if flag {
			arr = append(arr, strArr)
		}
	}
	marshal, _ := json.Marshal(arr)
	rm.RUnlock()
	w.Write(marshal)
}

func getCycleStatistics(w http.ResponseWriter, r *http.Request) {
	rm.RLock()
	defer rm.RUnlock()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	decoder := schema.NewDecoder()
	message := Message{}
	err := decoder.Decode(&message, r.URL.Query())
	if err != nil {
		log.Println(err)
		return
	}
	// 统计每个轮次的消息信息
	cycleTypeMap := make(map[byte]map[string]*MessageCycle)
	for msgType, _ := range msgIdMap {
		cycleMap := make(map[string]*MessageCycle)
		for k, v := range cacheMap {
			messageGroup := v.getMessagesByGroup(msgType, message)
			nodeCount := len(v.getNodes())
			if len(messageGroup) > 0 {
				cycle := staticticsCycle(messageGroup, nodeCount)
				cycleMap[k] = &cycle
			}
		}
		cycleTypeMap[msgType] = cycleMap
	}
	for b, m := range cycleTypeMap {
		if b == Graft || b == LazyPush {
			for s := range m {
				msg := cycleTypeMap[EagerPush][s]
				if msg != nil {
					msg.RMR = msg.RMR + m[s].RMR
				}
			}
		}
	}

	var builder strings.Builder
	marshal, _ := json.Marshal(cycleTypeMap)
	builder.WriteString(string(marshal))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(builder.String()))
}

func exportDataset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	dataSet := make(map[string][]Message)
	for k, v := range cacheMap {
		dataSet[k] = v.getMessages()
	}

	if _, err := os.Stat("./dataset"); os.IsNotExist(err) {
		err := os.MkdirAll("./dataset", os.ModePerm)
		if err != nil {
			fmt.Printf("创建文件夹失败: %v\n", err)
			return
		}
	}

	data, err := json.Marshal(dataSet)
	if err != nil {
		log.Fatalln(err)
	}
	err = os.WriteFile("./dataset/cacheMap.json", data, 0644)
	if err != nil {
		log.Fatalln(err)
	}

	data, err = json.Marshal(msgIdMap)
	if err != nil {
		log.Fatalln(err)
	}
	err = os.WriteFile("./dataset/msgIdMap.json", data, 0644)
	if err != nil {
		log.Fatalln(err)
	}
}

func loadDataset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	byteValue, err := os.ReadFile("./dataset/cacheMap.json")
	if err != nil {
		log.Fatalln(err)
	}
	dataSet := make(map[string][]Message)
	err = json.Unmarshal(byteValue, &dataSet)
	if err != nil {
		log.Fatalln(err)
	}
	for k, v := range dataSet {
		cacheMap[k] = new(MessageCache)
		cacheMap[k].messages = v
	}

	byteValue, err = os.ReadFile("./dataset/msgIdMap.json")
	if err != nil {
		log.Fatalln(err)
	}
	err = json.Unmarshal(byteValue, &msgIdMap)
	if err != nil {
		log.Fatalln(err)
	}
}
func index(w http.ResponseWriter, r *http.Request) {
	// 设置响应的内容类型为文本
	w.Header().Set("Content-Type", "text/plain")
	// 返回 "Hello, World"
	fmt.Fprintln(w, "Hello, World!")
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
	http.HandleFunc("/lack", lack)
	http.HandleFunc("/exportDataset", exportDataset)
	http.HandleFunc("/loadDataset", loadDataset)
	http.HandleFunc("/", index)
	fs := http.FileServer(http.Dir("./chart"))
	// 创建静态文件服务器

	// 使用 http.Handle 而不是 http.HandleFunc
	http.Handle("/chart/", http.StripPrefix("/chart/", fs))

	// 启动HTTP服务器
	fmt.Println("Server is running on http://localhost:8111")
	if err := http.ListenAndServe(":8111", nil); err != nil {
		fmt.Printf("Error starting server: %s\n", err)
	}
}

func main() {
	CreateWeb()
}
