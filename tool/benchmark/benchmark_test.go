package benchmark

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/schema"
)

var cacheMap map[string]*MessageCache

var num = 0
var startTime = time.Now()

func getMessageId() string {
	now := time.Now()
	if now.Sub(startTime).Milliseconds() > 500 {
		num++
	}
	startTime = now
	return fmt.Sprintf("%d", num)
}

// 接收节点广播的消息
func putRing(w http.ResponseWriter, r *http.Request) {
	decoder := schema.NewDecoder()
	message := Message{}
	err := decoder.Decode(&message, r.URL.Query())
	if err != nil {
		return
	}
	fmt.Println(message)
	message.Timestamp = int(time.Now().UnixMilli())
	message.Id = getMessageId()

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
	for _, v := range cacheMap {
		v.clearAll()
	}
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
	cycleMap := make(map[string]MessageCycle)
	for k, v := range cacheMap {
		cycle := MessageCycle{}
		cycle.Id = k
		cycle.BroadcastCount = len(v.getMessages())
		// 广播产生的总流量
		cycle.FlowSum = v.totalSize
		// 广播总时间
		cycle.LDT = v.endTime - v.startTime

		m := 0
		for _, message := range v.getMessages() {
			m += message.Size
		}
		n := len(v.getNodes())
		cycle.RMR = (float64(m) / (float64(n) - 1)) - 1

		// 统计有多少节点收到消息
		set := make(map[string]int)
		for _, m := range v.getMessages() {
			if _, e := set[m.From]; !e {
				set[m.From] = 0
			}
			set[m.From]++
		}
		cycle.Reliability = len(set)

		// 统计节点的扇入扇出流量方差
		nodeSet := make(map[string]*MessageNode)
		for _, m := range v.getMessages() {
			if _, e := nodeSet[m.From]; !e {
				nodeSet[m.From] = &MessageNode{Node: m.From, FlowIn: 0, FlowOut: m.Size}
			} else {
				nodeSet[m.From].FlowOut += m.Size
			}
			if _, e := nodeSet[m.Target]; !e {
				nodeSet[m.Target] = &MessageNode{Node: m.Target, FlowIn: m.Size, FlowOut: 0}
			} else {
				nodeSet[m.Target].FlowIn += m.Size
			}
		}

		if len(nodeSet) > 0 {
			flowInSum := 0
			flowOutSum := 0
			flowInAvg := 0.0
			flowOutAvg := 0.0
			flowInS := 0.0
			flowOutS := 0.0
			for _, v := range nodeSet {
				flowInSum += v.FlowIn
				flowOutSum += v.FlowOut
			}
			flowInAvg = float64(flowInSum) / float64(len(nodeSet))
			flowOutAvg = float64(flowOutSum) / float64(len(nodeSet))
			for _, v := range nodeSet {
				flowInS += math.Pow(float64(v.FlowIn)-flowInAvg, 2)
				flowOutS += math.Pow(float64(v.FlowOut)-flowOutAvg, 2)
			}
			flowInS = float64(flowInS) / float64(len(nodeSet))
			flowOutS = float64(flowOutS) / float64(len(nodeSet))
			cycle.FlowInS = flowInS
			cycle.FlowOutS = flowOutS
		}

		cycleMap[k] = cycle
	}

	var builder strings.Builder
	marshal, _ := json.Marshal(cycleMap)
	builder.WriteString(string(marshal))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(builder.String()))
}

func Test_server(t *testing.T) {
	t.Helper()
	cacheMap = make(map[string]*MessageCache)

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
