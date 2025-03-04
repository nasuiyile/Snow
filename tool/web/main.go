package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/schema"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var sm sync.Map
var m = make(map[string][]Message)
var totalMap = make(map[string]map[string]int, 0)
var mMutex sync.Mutex
var m2Mutex sync.Mutex
var title = ""
var rNode atomic.Int32

type Message struct {
	Size   int //多少个byte
	Target string
	From   string
}

func put(key string, value Message) {
	mMutex.Lock()
	defer mMutex.Unlock()
	_, exists := m[key]
	if exists {
		m[key] = append(m[key], value)
	} else {
		m[key] = []Message{value}
	}
}

// 处理GET请求的函数
func putRing(w http.ResponseWriter, r *http.Request) {
	decoder := schema.NewDecoder()
	data := Message{}
	err := decoder.Decode(&data, r.URL.Query())
	if err != nil {
		return
	}
	put(data.From, data)
}

// 处理GET请求的函数
func handleGetRequest(w http.ResponseWriter, r *http.Request) {
	// 解析URL中的查询参数
	queryParams := r.URL.Query()

	// 将查询参数转换为map
	//paramsMap := make(map[string]string)
	for key, values := range queryParams {
		// 如果同一个key有多个值，只取第一个值
		sm.Store(key, values[0])
	}

	sm.Range(func(key, value any) bool {
		// 将 value 从 string 转换为 int
		strValue, ok := value.(string) // 断言 value 是 string 类型
		if !ok {
			fmt.Printf("Key: %v, Value is not a string\n", key)
			return true // 继续遍历
		}

		intValue, err := strconv.Atoi(strValue) // 将 string 转换为 int
		if err != nil {
			fmt.Printf("Key: %v, Failed to convert value to int: %v\n", key, err)
			return true // 继续遍历
		}

		// 打印转换后的结果
		fmt.Printf("Key: %v, Value (int): %d\n", key, intValue)
		return true // 继续遍历
	})

	// 返回响应
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Query parameters received and converted to map!"))
}

func get(w http.ResponseWriter, r *http.Request) {

	// 将 sync.Map 转换为普通的 map
	data := make(map[string]interface{})
	sm.Range(func(key, value interface{}) bool {
		// 将键值对存储到普通的 map 中
		data[fmt.Sprint(key)] = value
		return true
	})

	// 将普通的 map 序列化为 JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Println("Error converting to JSON:", err)
		return
	}
	sm = sync.Map{}
	w.WriteHeader(http.StatusOK)
	w.Write(jsonData)
}
func counterSize(w http.ResponseWriter, r *http.Request) {
	m2 := make(map[string]int)
	totalSize := 0
	totalCount := 0
	for k, v := range m {
		num := 0
		for _, msg := range v {
			num += msg.Size
			totalCount++
		}
		totalSize += num
		m2[k] = num
	}
	m2["totalSize"] = totalSize
	m2["totalCount"] = totalCount
	jsonData, err := json.MarshalIndent(m2, "", "  ")
	if err != nil {
		fmt.Println("Error converting to JSON:", err)
		return
	}
	w.Write(jsonData)

}
func statistics() map[string]int {
	m2 := make(map[string]int)
	totalSize := 0
	totalCount := 0
	for k, v := range m {
		num := 0
		for _, msg := range v {
			num += msg.Size
			totalCount++
		}
		totalSize += num
		m2[k] = num
	}
	m2["totalSize"] = totalSize
	m2["totalCount"] = totalCount
	return m2

}
func totalCount(w http.ResponseWriter, r *http.Request) {
	//totalMap[title+"totoalCount"] = statistics()["totalCount"]
	//totalMap[title+"totalSize"] = statistics()["totalSize"]
	avgTotalCount := 0
	avgTotalSize := 0
	for _, v := range totalMap {
		avgTotalCount += v["totalCount"]
		avgTotalSize += v["avgTotalSize"]
	}
	m2 := make(map[string]int)
	m2["avg"] = 0
	m2["avgTotalCount"] = avgTotalCount
	m2["avgTotalSize"] = avgTotalSize
	totalMap["avg"] = m2
	jsonData, err := json.MarshalIndent(totalMap, "", "  ")

	if err != nil {
		fmt.Println("Error converting to JSON:", err)
		return
	}
	w.Write(jsonData)
	delete(totalMap, "avg")

}
func startTotalCount(w http.ResponseWriter, r *http.Request) {
	m2Mutex.Lock()
	defer m2Mutex.Unlock()
	if title == "" {
		title = r.URL.Query().Get("title")
	} else {
		m2 := statistics()
		m3 := make(map[string]int)
		m3[title] = m2[title]
		m3["totalSize"] = m2["totalSize"]
		m3["totalCount"] = m2["totalCount"]
		m3["receiveNode"] = int(rNode.Load())
		rNode.Store(0)
		totalMap[title] = m3
		title = r.URL.Query().Get("title")
		m = make(map[string][]Message)
		sm = sync.Map{}
	}

}
func reportMsg(w http.ResponseWriter, r *http.Request) {
	rNode.Add(1)
}

func getRing(w http.ResponseWriter, r *http.Request) {

	//// 将 sync.Map 转换为普通的 map
	//data := make(map[string]interface{})
	//sm.Range(func(key, value interface{}) bool {
	//    // 将键值对存储到普通的 map 中
	//    data[fmt.Sprint(key)] = value
	//    return true
	//})
	var builder strings.Builder
	builder.WriteString("graph G {layout=neato;")

	var arr = make([]string, 0)
	var scale float64 = float64(5)
	for k, _ := range m {
		arr = append(arr, k)
	}
	for _, v := range m {
		for _, data := range v {
			flag := false
			for _, value := range arr {
				if value == data.Target {
					flag = true
				}
			}
			if !flag {
				arr = append(arr, data.Target)
			}
		}
	}
	sort.Strings(arr)
	// 计算每个节点的角度
	angleIncrement := 2 * math.Pi / float64(len(arr))

	// 生成节点和边
	for i, label := range arr {
		// 计算节点在圆上的位置
		angle := float64(i) * angleIncrement
		x := scale * math.Cos(angle)
		y := scale * math.Sin(angle)

		// 生成节点
		builder.WriteString(fmt.Sprintf("    \"%s\" [pos=\"%.2f,%.2f!\"];\n", label, x, y))

		// 生成边，连接当前节点和下一个节点
		nextIndex := (i + 1) % len(arr)
		builder.WriteString(fmt.Sprintf("    \"%s\" -- \"%s\";\n", label, arr[nextIndex]))
	}
	//for i, v := range arr {
	//    if i == len(arr)-1 {
	//        builder.WriteString(v + " -- " + arr[0] + ";")
	//
	//    } else {
	//        builder.WriteString(v + " -- " + arr[i+1] + ";")
	//
	//    }
	//}

	//builder.WriteString(";")
	for k, v := range m {
		for _, target := range v {
			for _, v := range arr {
				if v == k {

				}
			}
			builder.WriteString("\"" + k + "\"" + " -- " + "\"" + target.Target + "\"" + " [dir=forward];")
		}
	}

	builder.WriteString("}")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(builder.String()))
	fmt.Println(m)
}
func clean(w http.ResponseWriter, r *http.Request) {
	m = make(map[string][]Message)
	sm = sync.Map{}
	totalMap = make(map[string]map[string]int, 0)

}
func main() {
	// 注册路由和处理函数
	http.HandleFunc("/api", handleGetRequest)
	http.HandleFunc("/get", get)
	http.HandleFunc("/putRing", putRing)
	http.HandleFunc("/getRing", getRing)
	http.HandleFunc("/clean", clean)
	http.HandleFunc("/counterSize", counterSize)
	http.HandleFunc("/totalCount", totalCount)
	http.HandleFunc("/startTotalCount", startTotalCount)
	http.HandleFunc("/reportMsg", reportMsg)
	// 启动HTTP服务器
	fmt.Println("Server is running on http://localhost:8111")
	if err := http.ListenAndServe(":8111", nil); err != nil {
		fmt.Printf("Error starting server: %s\n", err)
	}
}
