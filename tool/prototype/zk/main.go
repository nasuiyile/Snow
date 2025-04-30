package main

import (
	"errors"
	"fmt"
	"github.com/samuel/go-zookeeper/zk"
	"time"
)

func main() {
	fmt.Println("hello world")
	registerService([]string{"127.0.0.1:2181"}, "Snow", "127.0.0.1:20000")
	watchServices("Snow")
	select {}
}

func registerService(zkHosts []string, serviceName, serviceAddr string) {
	conn, _, err := zk.Connect(zkHosts, time.Second*5)
	if err != nil {
		panic(err)
	}
	// 先创建父节点（如果不存在）
	_, err = conn.Create("/services", []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && !errors.Is(err, zk.ErrNodeExists) {
		panic(err)
	}
	// 确保服务根节点存在
	rootPath := "/services/" + serviceName
	_, err = conn.Create(rootPath, []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && !errors.Is(err, zk.ErrNodeExists) {
		panic(err)
	}

	// 创建临时节点(服务实例)
	instancePath := rootPath + "/instance-"
	_, err = conn.CreateProtectedEphemeralSequential(instancePath, []byte(serviceAddr), zk.WorldACL(zk.PermAll))
	if err != nil {
		panic(err)
	}
	// 保持连接，节点会随连接断开而自动删除
	// 在实际应用中应该处理连接断开和重连逻辑
}

func discoverServices(zkHosts []string, serviceName string) ([]string, <-chan zk.Event) {
	conn, _, err := zk.Connect(zkHosts, time.Second*5)
	if err != nil {
		panic(err)
	}

	// 先创建父节点（如果不存在）
	_, err = conn.Create("/services", []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && !errors.Is(err, zk.ErrNodeExists) {
		panic(err)
	}
	servicePath := "/services/" + serviceName
	_, err = conn.Create(servicePath, []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && !errors.Is(err, zk.ErrNodeExists) {
		panic(err)
	}

	// 获取当前服务列表
	children, _, eventCh, err := conn.ChildrenW(servicePath)
	if err != nil {
		panic(err)
	}

	var addrs []string
	for _, child := range children {
		data, _, err := conn.Get(servicePath + "/" + child)
		if err != nil {
			continue
		}
		addrs = append(addrs, string(data))
	}

	return addrs, eventCh
}

// 监听服务变化
func watchServices(serviceName string) {
	addrs, eventCh := discoverServices([]string{"127.0.0.1:2181"}, serviceName)
	fmt.Println("Current services:", addrs)

	for {
		select {
		case event := <-eventCh:
			if event.Type == zk.EventNodeChildrenChanged {
				// 重新获取服务列表
				newAddrs, newEventCh := discoverServices([]string{"127.0.0.1:2181"}, serviceName)
				fmt.Println("Services changed:", newAddrs)
				eventCh = newEventCh
			}
		}
	}
}
