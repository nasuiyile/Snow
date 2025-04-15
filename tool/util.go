package tool

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/hashicorp/go-msgpack/v2/codec"
	"github.com/zeebo/blake3"
	"math"
	"math/rand"
	"net"
	. "snow/common"
	"sort"
	"strconv"
	"strings"
	"time"
)

// pushScale is the minimum number of nodes
// before we start scaling the push/pull timing. The scale
// effect is the log2(Nodes) - log2(pushScale). This means
// that the 33rd node will cause us to double the interval,
// while the 65th will triple it.
const pushScaleThreshold = 32

// KRandomNodes 定义一个函数，生成指定范围内的随机数，如果取到特定值则重新生成
func KRandomNodes(min int, max int, excludes []int, k int) []int {
	res := make([]int, 0)
	if max-min+1 <= k+len(excludes) {
		for ; min <= max; min++ {
			if !contains(min, excludes) {
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
		if contains(randomNum, res) {
			goto START

		}
		// 如果生成的随机数不等于 exclude，则返回
		if !contains(randomNum, excludes) {
			res = append(res, randomNum)

		}
		// 否则继续循环，重新生成
	}
	return res
}
func contains(target int, nums []int) bool {
	for _, v := range nums {
		if v == target {
			return true
		}
	}
	return false
}

func HashByte(msg []byte) []byte {
	sum256 := blake3.Sum256(msg)
	return sum256[:]

}
func Hash(msg []byte) string {
	sum256 := blake3.Sum256(msg)
	sum := string(sum256[:])
	return sum
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

// PushScale is used to scale the time interval at which push/pull
// syncs take place. It is used to prevent network saturation as the
// cluster size grows
func PushScale(interval time.Duration, n int) time.Duration {
	// Don't scale until we cross the threshold
	if n <= pushScaleThreshold {
		return interval
	}
	multiplier := math.Ceil(math.Log2(float64(n))-math.Log2(pushScaleThreshold)) + 1.0
	return time.Duration(multiplier) * interval
}

// 获取8个随机的byte值
func RandomNumber() []byte {
	// 生成一个随机的 int64（可能包含负数）
	randomInt64 := rand.Uint64()

	// 将 int64 转换为 8 个字节的切片
	bytes := make([]byte, 8)
	binary.BigEndian.PutUint64(bytes, uint64(randomInt64))

	return bytes
}

func CopyMsg(msg []byte) []byte {
	res := make([]byte, len(msg))
	copy(res, msg)
	return res
}
func RandInt(min, max int) int {
	if min >= max {
		panic("wrong starting value")
	}
	return rand.Intn(max-min) + min
}
func PackTagToHead(msgType MsgType, changeType MsgAction, msg []byte) []byte {
	data := make([]byte, len(msg)+TimeLen+TagLen)
	data[0] = msgType
	data[1] = changeType
	timeBytes := RandomNumber()
	copy(data[TagLen:], timeBytes)
	copy(data[TimeLen+TagLen:], msg)
	return data
}

func PackTag(msgType MsgType, changeType MsgAction) []byte {
	data := make([]byte, TimeLen+TagLen)
	data[0] = msgType
	data[1] = changeType
	timeBytes := RandomNumber()
	copy(data[TagLen:], timeBytes)
	return data
}

func CutBytes(bytes []byte) []byte {
	return bytes[Placeholder-TimeLen:]
}

func CutTimestamp(bytes []byte) []byte {
	return bytes[TimeLen:]
}

// RemoveElement 泛型版本，删除切片中的指定元素
func RemoveElement[T comparable](arr []T, val T) []T {
	result := []T{}
	for _, v := range arr {
		if v != val {
			result = append(result, v)
		}
	}
	return result
}

// DeleteAtIndexes 删除数组中指定索引位置的元素
func DeleteAtIndexes[T any](arr []T, indexes ...int) []T {
	if len(indexes) == len(arr) {
		return arr
	}

	// 原地删除元素
	for _, idx := range indexes {
		if idx >= 0 && idx < len(arr) {
			// 将后面的元素向前移动一位
			copy(arr[idx:], arr[idx+1:])
			// 缩短切片长度
			arr = arr[:len(arr)-1]
		}
	}

	return arr
}

func GetPortByIp(ip string) int {
	split := strings.Split(ip, ":")
	port, _ := strconv.Atoi(split[1])
	return port
}

// FindOrInsert 在有序的 byte slice 切片中查找或插入 target。
// 返回插入位置索引，以及是否进行了插入。
func FindOrInsert(list *[][]byte, target []byte) (int, bool) {
	index := sort.Search(len(*list), func(i int) bool {
		return bytes.Compare((*list)[i], target) >= 0
	})

	if index < len(*list) && bytes.Compare((*list)[index], target) == 0 {
		return index, false
	}

	*list = append(*list, nil)
	copy((*list)[index+1:], (*list)[index:])
	(*list)[index] = append([]byte{}, target...)

	return index, true
}

// Encode 直接支持打包完整的消息
func Encode(msgType MsgType, msgAction MsgAction, in interface{}, msgpackUseNewTimeFormat bool) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(msgType)   // 消息类型
	buf.WriteByte(msgAction) // 消息动作

	// 添加随机时间戳
	timeBytes := RandomNumber()
	buf.Write(timeBytes)

	// 使用 MsgPack 编码
	hd := codec.MsgpackHandle{
		BasicHandle: codec.BasicHandle{
			TimeNotBuiltin: !msgpackUseNewTimeFormat,
		},
	}
	enc := codec.NewEncoder(buf, &hd)
	err := enc.Encode(in)

	return buf.Bytes(), err
}

func DecodeMsgPayload(data []byte, out interface{}) error {
	// 检查数据长度是否足够
	if len(data) < TagLen+TimeLen+1 {
		return fmt.Errorf("DecodeMsgPayload: data too short (len=%d)", len(data))
	}

	// 跳过 TagLen + TimeLen，从实际消息内容开始解析
	msgpackData := data[TagLen+TimeLen:]

	var handle codec.MsgpackHandle
	dec := codec.NewDecoderBytes(msgpackData, &handle)
	return dec.Decode(out)
}

func IsLastDigitEqual(a, b int) bool {
	// 获取a的个位数和十位数
	lastDigitA := a % 10
	secondLastDigitA := (a / 10) % 10

	// 获取b的个位数和十位数
	lastDigitB := b % 10
	secondLastDigitB := (b / 10) % 10

	// 比较个位数和十位数是否都相等
	return lastDigitA == lastDigitB && secondLastDigitA == secondLastDigitB
}

// IntHash 计算一个整数的哈希值
func IntHash(n int) int {
	// 使用乘法哈希算法
	// 选择一个大的质数作为乘数
	const multiplier = 2654435761 // 32位乘法器 (来自Knuth)

	// 进行乘法并取高位作为哈希值
	hash := uint32(n) * multiplier

	// 返回int类型的哈希值
	return int(hash)
}
