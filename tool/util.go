package tool

import (
	"encoding/binary"
	"fmt"
	"github.com/zeebo/blake3"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// pushPullScale is the minimum number of nodes
// before we start scaling the push/pull timing. The scale
// effect is the log2(Nodes) - log2(pushPullScale). This means
// that the 33rd node will cause us to double the interval,
// while the 65th will triple it.
const pushPullScaleThreshold = 32

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

// KRandomNodes 定义一个函数，生成指定范围内的随机数，如果取到特定值则重新生成
func KRandomNodes(min, max, exclude int, k int) []int {
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
func BytesToTime(data []byte) int64 {
	return int64(binary.BigEndian.Uint64(data))
}

func IPv4To6Bytes(ipPort string) []byte {
	// 解析 IP 和 Port
	ipStr, portStr, err := splitIPPort(ipPort)
	if err != nil {
		return nil
	}
	// 将 IP 转换为字节数组
	ip := net.ParseIP(ipStr).To4()
	if ip == nil {
		return nil
	}
	// 将 Port 转换为整数
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return nil
	}
	// 构造结果数组
	var result [6]byte
	copy(result[:4], ip)            // 前 4 字节是 IP
	result[4] = byte(port >> 8)     // 端口高 8 位
	result[5] = byte(port & 0x00FF) // 端口低 8 位
	return result[:]
}

// 辅助函数：解析 IP 和 Port
func splitIPPort(ipPort string) (string, string, error) {
	host, port, _ := net.SplitHostPort(ipPort)
	return host, port, nil
}

func ByteToIPv4Port(data []byte) string {
	// 提取 IP 地址 (前 4 个字节)
	ip := net.IPv4(data[0], data[1], data[2], data[3])

	// 提取端口号 (后 2 个字节, 大端序)
	port := binary.BigEndian.Uint16(data[4:])

	// 构造 IP:Port 字符串
	return fmt.Sprintf("%s:%d", ip.String(), port)
}

// pushPushScale is used to scale the time interval at which push/pull
// syncs take place. It is used to prevent network saturation as the
// cluster size grows
func PushPullScale(interval time.Duration, n int) time.Duration {
	// Don't scale until we cross the threshold
	if n <= pushPullScaleThreshold {
		return interval
	}
	multiplier := math.Ceil(math.Log2(float64(n))-math.Log2(pushPullScaleThreshold)) + 1.0
	return time.Duration(multiplier) * interval
}
