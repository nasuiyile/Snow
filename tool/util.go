package tool

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"time"
)

func SendHttp(from string, target string, data []byte) {
	values := url.Values{}
	values.Add("From", from)
	values.Add("Target", target)
	values.Add("Size", fmt.Sprintf("%d", len(data)))

	// 构建完整的URL，包括查询参数
	baseURL := "http://127.0.0.1:8111/putRing"
	fullURL := fmt.Sprintf("%s?%s", baseURL, values.Encode())

	// 发送HTTP GET请求
	http.Get(fullURL)
}

// GetRandomExcluding 定义一个函数，生成指定范围内的随机数，如果取到特定值则重新生成
func GetRandomExcluding(min, max, exclude int) int {
	// 使用当前时间的纳秒级时间戳创建一个新的随机数生成器
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	for {
		// 生成 [min, max] 范围内的随机数
		randomNum := r.Intn(max-min+1) + min
		// 如果生成的随机数不等于 exclude，则返回
		if randomNum != exclude {
			return randomNum
		}
		// 否则继续循环，重新生成
	}
}
