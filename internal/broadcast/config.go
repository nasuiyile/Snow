package broadcast

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"snow/tool"
	"strconv"
	"strings"
	"time"
)

const TimeLen = 8
const TagLen = 2
const HashLen = 32

type Config struct {
	Ipv6             bool   `yaml:"Ipv6"`
	FanOut           int    `yaml:"FanOut"`
	LocalAddress     string `yaml:"LocalAddress"`
	Coloring         bool   `yaml:"Coloring"`
	Test             bool   `yaml:"Test"`
	ExpirationTime   int64  `yaml:"ExpirationTime"`
	ClientPortOffset int    `yaml:"ClientPortOffset"`
	ClientAddress    string
	PushPullInterval int64 `yaml:"PushPullInterval"`
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
		return tool.IPv4To6Bytes(c.LocalAddress)
	}
	return nil
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
	config.ExpirationTime = config.ExpirationTime * int64(time.Second)
	config.PushPullInterval = config.PushPullInterval * int64(time.Second)
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
