package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	. "snow/common"
	"snow/util"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/gorilla/schema"
)

var cacheMap map[int]*MessageCache
var msgIdMap map[byte]map[string]int
var rm sync.RWMutex
var chartPath string

func getMessageCycle(m Message) int {
	rm.Lock()
	defer rm.Unlock()

	if _, e := msgIdMap[m.MsgType]; !e {
		msgIdMap[m.MsgType] = make(map[string]int)
	}
	if _, e := msgIdMap[m.MsgType][m.Id]; !e {
		msgIdMap[m.MsgType][m.Id] = len(msgIdMap[m.MsgType]) + 1
	}
	num := msgIdMap[m.MsgType][m.Id]
	return num
}

// 接收节点广播的消息
func putRing(w http.ResponseWriter, r *http.Request) {
	decoder := schema.NewDecoder()
	message := Message{}
	err := decoder.Decode(&message, r.URL.Query())
	if err != nil {
		log.Error(err)
		return
	}
	//fmt.Println(message)
	message.Timestamp = int(time.Now().UnixMilli())

	idBts := []byte(message.Id)
	ids := make([]string, 0)
	for _, bt := range idBts {
		ids = append(ids, fmt.Sprintf("%d", bt))
	}
	message.Id = strings.Join(ids, "")

	message.Cycle = getMessageCycle(message)
	rm.Lock()
	if _, exisit := cacheMap[message.Cycle]; !exisit {
		cacheMap[message.Cycle] = CreateMessageCache()
	}
	cacheMap[message.Cycle].put(message)
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
	messageMap := make(map[int][]Message)

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

	nodeMap := make(map[int][]*MessageNode, 0)

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
	cacheMap = make(map[int]*MessageCache)
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
			strArr = append(strArr, fmt.Sprintf("%s%d", "127.0.0.1:", i+20000))
		}
		for _, msg := range v.messages {
			if msg.MsgType == RegularMsg {
				flag = true
			}
			strArr = util.RemoveElement(strArr, msg.Target)
		}
		if flag {
			arr = append(arr, strArr)
		}
	}
	marshal, _ := json.Marshal(arr)
	rm.RUnlock()
	w.Write(marshal)
}

func getCycleTypeStatistics(message Message) map[byte]map[int]*MessageCycle {
	cycleTypeMap := make(map[byte]map[int]*MessageCycle)
	for msgType, _ := range msgIdMap {
		cycleMap := make(map[int]*MessageCycle)
		// 遍历轮次
		for k, v := range cacheMap {
			// 条件查找 msgType、Num、FanOut
			messageGroup := v.getMessagesByGroup(msgType, message)
			if len(messageGroup) > 0 {
				cycle := staticticsCycle(messageGroup)
				cycle.Cycle = k
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
	return cycleTypeMap
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
	cycleTypeMap := getCycleTypeStatistics(message)

	var builder strings.Builder
	marshal, _ := json.Marshal(cycleTypeMap)
	builder.WriteString(string(marshal))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(builder.String()))
}

func getNumFanoutStatistics(w http.ResponseWriter, r *http.Request) {
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
	cycleTypeMap := getCycleTypeStatistics(message)

	typeData := make(map[byte]float64, 0)
	for msgType, v := range cycleTypeMap {
		LDT_sum := 0
		RMR_sum := 0.0
		Reliability_sum := 0.0
		n := 0
		for _, message := range v {
			n++
			LDT_sum += message.LDT
			RMR_sum += message.RMR
			Reliability_sum += message.Reliability
		}
		if n == 0 {
			typeData[msgType] = 0.0
		} else {
			typeData[msgType] = 0.0
		}
	}

	var builder strings.Builder
	marshal, _ := json.Marshal(cycleTypeMap)
	builder.WriteString(string(marshal))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(builder.String()))
}

func exportDatasetOld(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	cacheData := make(map[string]string)

	dataSet := make(map[int][]Message)
	for k, v := range cacheMap {
		dataSet[k] = v.getMessages()
	}
	data, err := json.Marshal(dataSet)
	if err != nil {
		log.Println("export dataSet", err)
		return
	}
	cacheData["dataSet"] = string(data)

	data, err = json.Marshal(msgIdMap)
	if err != nil {
		log.Println("export Marshal", err)
		return
	}
	cacheData["msgIdMap"] = string(data)

	data, err = json.Marshal(cacheData)
	if err != nil {
		log.Println("export Marshal", err)
		return
	}

	h := w.Header()
	h.Set("Content-type", "application/octet-stream")
	h.Set("CharacterEncoding", "utf-8")
	h.Set("Content-Disposition", "attachment;filename=cacheData.json")
	w.Write(data)
}

func exportDataset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	dataSet := make(map[int][]Message)
	for k, v := range cacheMap {
		dataSet[k] = v.getMessages()
	}
	data, err := json.Marshal(dataSet)
	if err != nil {
		log.Println("export dataSet", err)
		return
	}

	h := w.Header()
	h.Set("Content-type", "application/octet-stream")
	h.Set("CharacterEncoding", "utf-8")
	h.Set("Content-Disposition", "attachment;filename=cacheData.json")
	w.Write(data)
}

func exportDatasetCsv(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	lines := new(bytes.Buffer)
	for _, v := range cacheMap {
		for _, message := range v.getMessages() {
			line := make([]string, 10)
			line[0] = message.Id
			line[1] = fmt.Sprintf("%d", message.Cycle)
			line[2] = fmt.Sprintf("%d", message.Size)
			line[3] = message.Target
			line[4] = message.From
			line[5] = fmt.Sprintf("%d", message.Timestamp)
			line[6] = fmt.Sprintf("%d", message.MsgType)
			line[7] = fmt.Sprintf("%d", message.Num)
			line[8] = fmt.Sprintf("%d", message.FanOut)
			line[9] = "\n"
			str := strings.Join(line, ",")
			lines.WriteString(str)
		}
	}

	h := w.Header()
	h.Set("Content-type", "application/octet-stream")
	h.Set("CharacterEncoding", "utf-8")
	h.Set("Content-Disposition", "attachment;filename=cacheData.csv")
	w.Write([]byte(lines.Bytes()))
}

func exportDatasetAndClose(w http.ResponseWriter, r *http.Request) {
	lines := new(bytes.Buffer)
	for _, v := range cacheMap {
		for _, message := range v.getMessages() {
			line := make([]string, 10)
			line[0] = message.Id
			line[1] = fmt.Sprintf("%d", message.Cycle)
			line[2] = fmt.Sprintf("%d", message.Size)
			line[3] = message.Target
			line[4] = message.From
			line[5] = fmt.Sprintf("%d", message.Timestamp)
			line[6] = fmt.Sprintf("%d", message.MsgType)
			line[7] = fmt.Sprintf("%d", message.Num)
			line[8] = fmt.Sprintf("%d", message.FanOut)
			line[9] = "\n"
			str := strings.Join(line, ",")
			lines.WriteString(str)
		}
	}

	// 生成当前时间戳文件名
	now := time.Now()
	// 格式化时间为：2025-04-18_15-30-45
	timestamp := now.Format("2006-01-02_15-04-05")
	// 使用格式化时间作为文件名
	filename := fmt.Sprintf("dataset/%s.json", timestamp)

	// 写入文件
	err := os.WriteFile(filename, []byte(lines.Bytes()), 0644)
	if err != nil {
		log.Println("failed to write file", err)
		return
	}

	log.Printf("Data exported to %s\n", filename)

}

func loadDataset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	file, _, err := r.FormFile("file")
	if err != nil {
		log.Println("load FormFile", err)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		log.Println("load ReadAll", err)
		return
	}
	cacheData := make(map[int][]Message)
	err = json.Unmarshal(data, &cacheData)
	if err != nil {
		log.Println("load cacheData", err)
		return
	}

	for i := range len(cacheData) {
		for _, message := range cacheData[i+1] {
			message.Cycle = getMessageCycle(message)
			rm.Lock()
			if _, exisit := cacheMap[message.Cycle]; !exisit {
				cacheMap[message.Cycle] = CreateMessageCache()
			}
			cacheMap[message.Cycle].put(message)
			rm.Unlock()
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}

func loadDatasetCsv(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	file, _, err := r.FormFile("file")
	if err != nil {
		log.Println("load FormFile", err)
		return
	}
	defer file.Close()
	reader := bufio.NewReader(file)

	messageCycle := make(map[int][]Message)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		row := strings.Split(line, ",")

		message := Message{}
		message.Id = row[0]
		message.Cycle, _ = strconv.Atoi(row[1])
		message.Size, _ = strconv.Atoi(row[2])
		message.Target = row[3]
		message.From = row[4]
		message.Timestamp, _ = strconv.Atoi(row[5])
		msgType, _ := strconv.Atoi(row[6])
		message.MsgType = byte(msgType)
		message.Num, _ = strconv.Atoi(row[7])
		message.FanOut, _ = strconv.Atoi(row[8])

		if _, e := messageCycle[message.Cycle]; !e {
			messageCycle[message.Cycle] = make([]Message, 0)
		}
		messageCycle[message.Cycle] = append(messageCycle[message.Cycle], message)
	}

	for i := range len(messageCycle) {
		for _, message := range messageCycle[i+1] {
			message.Cycle = getMessageCycle(message)
			rm.Lock()
			if _, exisit := cacheMap[message.Cycle]; !exisit {
				cacheMap[message.Cycle] = CreateMessageCache()
			}
			cacheMap[message.Cycle].put(message)
			rm.Unlock()
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}

func loadDatasetOld(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	file, _, err := r.FormFile("file")
	if err != nil {
		log.Println("load FormFile", err)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		log.Println("load ReadAll", err)
		return
	}
	cacheData := make(map[string]string)
	err = json.Unmarshal(data, &cacheData)
	if err != nil {
		log.Println("load cacheData", err)
		return
	}

	msgIdMapStr := cacheData["msgIdMap"]
	idSet := make(map[byte]map[string]int, 0)
	err = json.Unmarshal([]byte(msgIdMapStr), &idSet)
	if err != nil {
		log.Println("load msgIdMap", err)
		return
	}

	idRevertSet := make(map[int]map[byte]string, 0)
	for msgType, v := range idSet {
		for oldId, cycle := range v {
			if _, e := idRevertSet[cycle]; !e {
				idRevertSet[cycle] = make(map[byte]string, 0)
			}
			idRevertSet[cycle][msgType] = oldId
		}
	}

	dataSetStr := cacheData["dataSet"]
	dataSet := make(map[int][]Message)
	err = json.Unmarshal([]byte(dataSetStr), &dataSet)
	if err != nil {
		log.Println("load dataSet", err)
		return
	}

	for i := range len(dataSet) {
		for _, message := range dataSet[i+1] {
			message.Id = idRevertSet[i+1][message.MsgType]
		}
	}

	for cycle, v := range dataSet {
		for _, message := range v {
			message.Id = idRevertSet[cycle][message.MsgType]
		}
	}

	for i := range len(dataSet) {
		for _, message := range dataSet[i+1] {
			message.Cycle = getMessageCycle(message)
			rm.Lock()
			if _, exisit := cacheMap[message.Cycle]; !exisit {
				cacheMap[message.Cycle] = CreateMessageCache()
			}
			cacheMap[message.Cycle].put(message)
			rm.Unlock()
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}

func goChart1(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles(fmt.Sprintf("%s%s", chartPath, "go_cycle_statistics.html"))
	if err != nil {
		fmt.Println("go chart, err:", err)
		return
	}
	info := []byte{RegularMsg, ColoringMsg, GossipMsg, EagerPush}
	tmpl.Execute(w, info)
}

func goChart2(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles(fmt.Sprintf("%s%s", chartPath, "go_num_fanout_statistics.html"))
	if err != nil {
		fmt.Println("go chart, err:", err)
		return
	}
	info := []byte{RegularMsg, ColoringMsg, GossipMsg, EagerPush}
	tmpl.Execute(w, info)
}

func index(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello, World!"))
}

var server http.Server

func CreateWeb() {
	var mux = http.NewServeMux()
	cacheMap = make(map[int]*MessageCache)
	msgIdMap = make(map[byte]map[string]int)
	chartPath = "./tool/benchmark/chart/"
	// chartPath = "./chart/"
	// 注册路由和处理函数
	mux.HandleFunc("/putRing", putRing)
	mux.HandleFunc("/getRing", getRing)
	mux.HandleFunc("/totalCount", totalCount)
	mux.HandleFunc("/clean", clean)
	mux.HandleFunc("/getAllMessage", getAllMessage)
	mux.HandleFunc("/getAllNode", getAllNode)
	mux.HandleFunc("/getNodeStatistics", getNodeStatistics)
	mux.HandleFunc("/getCycleStatistics", getCycleStatistics)
	mux.HandleFunc("/getNumFanoutStatistics", getNumFanoutStatistics)
	mux.HandleFunc("/lack", lack)
	mux.HandleFunc("/exportDataset", exportDataset)
	mux.HandleFunc("/exportDatasetCsv", exportDatasetCsv)
	mux.HandleFunc("/exportDatasetOld", exportDatasetOld)
	mux.HandleFunc("/loadDataset", loadDataset)
	mux.HandleFunc("/loadDatasetOld", loadDatasetOld)
	mux.HandleFunc("/loadDatasetCsv", loadDatasetCsv)
	mux.HandleFunc("/goChart", goChart1)
	mux.HandleFunc("/goChart2", goChart2)
	mux.HandleFunc("/", index)
	fs := http.FileServer(http.Dir(chartPath))
	// 创建静态文件服务器
	// 使用 http.Handle 而不是 http.HandleFunc
	http.Handle("/chart/", http.StripPrefix("/chart/", fs))

	// 启动HTTP服务器
	fmt.Println("Server is running on http://localhost:8111")
	server := &http.Server{
		Addr:    ":8111",
		Handler: mux,
	}
	// 创建一个 channel，用于通知主线程退出
	shutdownChan := make(chan struct{})

	// 注册 /shutdown 路由（必须先声明 server 才能用）
	mux.HandleFunc("/exportDatasetAndClose", func(w http.ResponseWriter, r *http.Request) {
		go func() {
			exportDatasetAndClose(w, r)
			//fmt.Println("Received shutdown request...")
			//// 你也可以用 Shutdown(context.TODO()) 实现优雅退出
			//if err := server.Close(); err != nil {
			//	fmt.Println("Error during shutdown:", err)
			//}
			//close(shutdownChan) // 通知主线程退出
		}()
		fmt.Fprintln(w, "Server is shutting down...")
	})

	// 启动服务
	go func() {
		fmt.Println("Server is running on http://localhost:8111")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Println("Server error:", err)
		}
	}()

	// 阻塞主线程，直到 shutdownChan 被关闭
	<-shutdownChan
	fmt.Println("Server exited.")
}

func main() {
	CreateWeb()
}
