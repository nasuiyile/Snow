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
	Port             int           `yaml:"Port"`
	Ipv6             bool          `yaml:"Ipv6"`
	FanOut           int           `yaml:"FanOut"`
	LocalAddress     string        `yaml:"LocalAddress"`
	Coloring         bool          `yaml:"Coloring"`
	Test             bool          `yaml:"Test"`
	ExpirationTime   time.Duration `yaml:"ExpirationTime"`
	ClientPortOffset int           `yaml:"ClientPortOffset"`
	ClientAddress    string
	ServerAddress    string
	PushPullInterval time.Duration `yaml:"PushPullInterval"`
	TCPTimeout       time.Duration `yaml:"TCPTimeout"`
	InitialServer    string        `yaml:"InitialServer"`
	DefaultServer    []string
	DefaultAddress   string `yaml:"DefaultAddress"`
	RemoteHttp       string `yaml:"RemoteHttp"`
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
		return tool.IPv4To6Bytes(c.ServerAddress)
	}
	return nil
}

type ConfigOption func(c *Config)

func NewConfig(filename string, opts ...ConfigOption) (*Config, error) {
	config, err := LoadConfig(filename)
	if err != nil {
		return nil, err
	}
	if err != nil {
		panic(err)
	}
	for _, action := range opts {
		action(config)
	}
	config.DefaultServer = strings.Split(config.DefaultAddress, ",")
	config.ClientAddress = fmt.Sprintf("%s:%d", config.LocalAddress, config.Port+config.ClientPortOffset)
	config.ServerAddress = fmt.Sprintf("%s:%d", config.LocalAddress, config.Port)
	tool.RemoteHttp = config.RemoteHttp
	return config, nil
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
	return fmt.Sprintf("%s:%d", split[0], port-c.ClientPortOffset)
}
