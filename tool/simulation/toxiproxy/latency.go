package toxiproxy

import (
	"fmt"
	toxiproxy "github.com/Shopify/toxiproxy/v2/client"
)

func AddLatency(startPort int, EndPort int) {

	// 创建Toxiproxy客户端
	toxiproxyClient := toxiproxy.NewClient("localhost:8474")
	// 创建一个新的代理
	for i := startPort; i <= EndPort; i++ {
		proxyName := fmt.Sprintf("my_proxy_%d", i)
		listen := fmt.Sprintf("127.0.0.1:%d", i+2000)
		upstream := fmt.Sprintf("127.0.0.1:%d", i)
		proxy, err := toxiproxyClient.CreateProxy(proxyName, listen, upstream)
		if err != nil {
			fmt.Println("Error creating proxy:", err)
			return
		}
		// 添加毒性规则 - 带宽限制
		_, err = proxy.AddToxic("latency_down", "latency", "downstream", 1.0, map[string]interface{}{
			"latency":              1,                  // 1毫秒延迟
			"jitter":               1,                  // 1毫秒抖动
			"rate":                 1024 * 1024 * 1024, // 千兆网
			"packet_loss":          0.001,              // 0.5%丢包率
			"latency_distribution": "normal",           // 正态分布
			"buffer_size":          1024 * 1024,        // 1MB缓冲区
		})
		if err != nil {
			fmt.Println("Error adding bandwidth toxic:", err)
			return
		}
	}
}
