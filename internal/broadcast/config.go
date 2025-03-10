package broadcast

import (
	"encoding/binary"
	"fmt"
	"gopkg.in/yaml.v3"
	"net"
	"os"
	"strconv"
	"strings"
)

const TimeLen = 8
const TagLen = 2

type Config struct {
	Ipv6             bool   `yaml:"Ipv6"`
	FanOut           int    `yaml:"FanOut"`
	LocalAddress     string `yaml:"LocalAddress"`
	Coloring         bool   `yaml:"Coloring"`
	Test             bool   `yaml:"Test"`
	ExpirationTime   int64  `yaml:"ExpirationTime"`
	ClientPortOffset int    `yaml:"ClientPortOffset"`
	ClientAddress    string
}

// CutBytes 这个方法会留下时间戳
func (c *Config) CutBytes(bytes []byte) []byte {
	return bytes[c.Placeholder()-8:]
}

func (c *Config) CutTimestamp(bytes []byte) []byte {
	return bytes[8:]
}

func (c *Config) Placeholder() int {
	//ipv4/6的地址和1个tag，加上8个byte的时间戳
	if c.Ipv6 {
		return 1 + 1 + 18 + 18 + 8
	} else {
		return 1 + 1 + 6 + 6 + 8
	}
}

func (c *Config) IpLen() int {
	if c.Ipv6 {
		return 18
	} else {
		return 6
	}
}
func (c *Config) IPBytes() []byte {
	if !c.Ipv6 {
		return IPv4To6Bytes(c.LocalAddress)
	}
	return nil
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

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Config) GetReliableTimeOut() int64 {
	return 60
}

func (c *Config) GetServerIp(clientIp string) string {
	split := strings.Split(clientIp, ":")
	port, _ := strconv.Atoi(split[1])
	return fmt.Sprintf("127.0.0.1:%d", port-c.ClientPortOffset)
}
