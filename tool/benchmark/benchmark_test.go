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

type Statistics struct {
	Duration map[string]int
	Avg      float64
	S        float64
}

func getNodeStatistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	nodeArr := make([]*MessageNode, 0)
	for _, v := range cacheMap {
		nodeArr = append(nodeArr, v.getNodes()...)
	}

	nodeMap := make(map[string][]MessageNode, 0)
	for _, node := range nodeArr {
		if _, e := nodeMap[node.Node]; !e {
			nodeMap[node.Node] = make([]MessageNode, 0)
		}
		nodeMap[node.Node] = append(nodeMap[node.Node], *node)
	}

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

// 计算扇入扇出方差
func getStatistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	durationMap := make(map[string]int)
	sum := 0.0
	avg := 0.0
	s := 0.0
	for k, v := range cacheMap {
		sub := v.endTime - v.startTime
		durationMap[k] = sub
		sum += float64(sub)
	}

	if len(durationMap) > 0 {
		avg = sum / float64(len(durationMap))
		for _, v := range durationMap {
			s += math.Pow(float64(v)-avg, 2)
		}
		s = s / float64(len(durationMap))
	}

	v := Statistics{Duration: durationMap, Avg: avg, S: s}
	jsonData, err := json.Marshal(v)
	if err != nil {
		fmt.Println("Error converting to JSON:", jsonData)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
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
	http.HandleFunc("/getStatistics", getStatistics)
	http.HandleFunc("/getNodeStatistics", getNodeStatistics)

	// 启动HTTP服务器
	fmt.Println("Server is running on http://localhost:8111")
	if err := http.ListenAndServe(":8111", nil); err != nil {
		fmt.Printf("Error starting server: %s\n", err)
	}
}
