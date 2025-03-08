package tool

import (
	"encoding/binary"
	"fmt"
	"github.com/zeebo/blake3"
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
	//if fanOut {
	//	values.Add("FanOut", "true")
	//} else {
	//	values.Add("FanOut", "false")
	//
	//}
	// 构建完整的URL，包括查询参数
	baseURL := "http://127.0.0.1:8111/putRing"
	fullURL := fmt.Sprintf("%s?%s", baseURL, values.Encode())

	// 发送HTTP GET请求
	http.Get(fullURL)
}

// GetRandomExcluding 定义一个函数，生成指定范围内的随机数，如果取到特定值则重新生成
func GetRandomExcluding(min, max, exclude int, k int) []int {
	res := make([]int, 0)
	if max-min+1 <= k+1 {
		for ; min <= max; min++ {
			if min != exclude {
				res = append(res, min)
			}
		}
		return res
	}
	// 使用当前时间的纳秒级时间戳创建一个新的随机数生成器
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
START:
	for len(res) < k {
		// 生成 [min, max] 范围内的随机数
		randomNum := r.Intn(max-min+1) + min
		for i := 0; i < len(res); i++ {
			if randomNum == res[i] {
				goto START
			}
		}
		// 如果生成的随机数不等于 exclude，则返回
		if randomNum != exclude {
			res = append(res, randomNum)
		}
		// 否则继续循环，重新生成
	}
	return res
}

func Hash(msg []byte) string {
	sum256 := blake3.Sum256([]byte(msg))
	sum := string(sum256[:])
	return sum
}

func TimeBytes() []byte {
	unix := time.Now().Unix()
	timestamp := make([]byte, 8)
	binary.BigEndian.PutUint64(timestamp, uint64(unix))
	return timestamp
}
