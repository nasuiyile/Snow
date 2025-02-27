package main

type Config struct {
	Ipv6 bool
}

func NewConfig() *Config {
	return &Config{
		Ipv6: false,
	}
}

func (c *Config) Placeholder() int {
	if c.Ipv6 {
		return 18 + 18 + 1
	} else {
		return 6 + 6 + 1
	}
}
