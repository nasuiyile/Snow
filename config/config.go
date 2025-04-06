package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"snow/common"
	"snow/tool"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port              int           `yaml:"Port"`
	Ipv6              bool          `yaml:"Ipv6"`
	FanOut            int           `yaml:"FanOut"`
	LocalAddress      string        `yaml:"LocalAddress"`
	Coloring          bool          `yaml:"Coloring"`
	Test              bool          `yaml:"Test"`
	ExpirationTime    time.Duration `yaml:"ExpirationTime"`
	ClientPortOffset  int           `yaml:"ClientPortOffset"`
	ClientAddress     string
	ServerAddress     string
	PushPullInterval  time.Duration `yaml:"PushPullInterval"`
	TCPTimeout        time.Duration `yaml:"TCPTimeout"`
	InitialServer     string        `yaml:"InitialServer"`
	DefaultServer     []string
	DefaultAddress    string        `yaml:"DefaultAddress"`
	RemoteHttp        string        `yaml:"RemoteHttp"`
	Report            bool          `yaml:"Report"`
	HeartbeatInterval time.Duration `yaml:"HeartbeatInterval"`
	IndirectChecks    int           `yaml:"IndirectChecks"`
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
		panic(err)
	}
	config.DefaultServer = strings.Split(config.DefaultAddress, ",")

	tool.RemoteHttp = config.RemoteHttp
	for _, action := range opts {
		action(config)
	}
	config.ClientAddress = fmt.Sprintf("%s:%d", config.LocalAddress, config.Port+config.ClientPortOffset)
	config.ServerAddress = fmt.Sprintf("%s:%d", config.LocalAddress, config.Port)
	common.Offset = config.ClientPortOffset
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
