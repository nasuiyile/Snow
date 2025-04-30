package broadcast

import (
	"errors"
	"github.com/samuel/go-zookeeper/zk"
	log "github.com/sirupsen/logrus"
	"snow/config"
	"snow/internal/membership"
	"sync"

	"time"
)

const prefix = "services"
const name = "Snow"

type Zookeeper struct {
	conn   *zk.Conn
	once   sync.Once
	Member *membership.MemberShipList
	stopCh chan struct{}
	config *config.Config
}

func StartZk(config *config.Config, m *membership.MemberShipList) *Zookeeper {

	z := &Zookeeper{Member: m, config: config}
	z.registerService(config.ZookeeperAddr, config.ServerAddress)
	z.watchServices(config.ZookeeperAddr)
	return z
}
func (z *Zookeeper) getConnect(zkHosts []string) *zk.Conn {
	if z.conn == nil {
		z.once.Do(func() { // 使用sync.Once确保只执行一次
			conn, _, err := zk.Connect(zkHosts, time.Second*5)
			if err != nil {
				panic(err)
			}
			z.conn = conn
		})
	}
	return z.conn
}

func (z *Zookeeper) registerService(zkHosts []string, serviceAddr string) {
	conn := z.getConnect(zkHosts)
	// 先创建父节点（如果不存在）
	_, err := conn.Create("/"+prefix, []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && !errors.Is(err, zk.ErrNodeExists) {
		log.Error(z.config.ServerAddress, "Failed to create parent node:", err)
		panic(err)
	}
	// 确保服务根节点存在
	rootPath := "/" + prefix + "/" + name
	_, err = conn.Create(rootPath, []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && !errors.Is(err, zk.ErrNodeExists) {
		panic(err)
	}
	// 创建临时顺序节点,在close的时候会自动删除
	instancePath := rootPath + "/instance-"
	_, err = conn.CreateProtectedEphemeralSequential(instancePath, []byte(serviceAddr), zk.WorldACL(zk.PermAll))
	if err != nil {
		panic(err)
	}
	// 保持连接，节点会随连接断开而自动删除
	// 在实际应用中应该处理连接断开和重连逻辑
}

// 监听服务变化
func (z *Zookeeper) watchServices(zkHosts []string) {

	addrs, eventCh := z.discoverServices(zkHosts)
	z.Member.UpdateMember(addrs)
	log.Println("Current services:", addrs)
	go func() {
		for {
			select {
			case event := <-eventCh:
				if event.Type == zk.EventNodeChildrenChanged {
					// 重新获取服务列表
					newAddrs, newEventCh := z.discoverServices(zkHosts)
					z.Member.UpdateMember(newAddrs)
					log.Debugf("Services changed:", newAddrs)
					eventCh = newEventCh
				}
			case <-z.stopCh:
				return
			}
		}
	}()
}
func (z *Zookeeper) Stop() {

	if z.stopCh != nil {
		close(z.stopCh)
	}
	if z.conn != nil {
		z.conn.Close()
	}
}

func (z *Zookeeper) discoverServices(zkHosts []string) ([]string, <-chan zk.Event) {
	conn := z.getConnect(zkHosts)
	// 先创建父节点（如果不存在）
	_, err := conn.Create("/"+prefix, []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && !errors.Is(err, zk.ErrNodeExists) && !errors.Is(err, zk.ErrClosing) {
		panic(err)
	}
	servicePath := "/" + prefix + "/" + name
	_, err = conn.Create(servicePath, []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && !errors.Is(err, zk.ErrNodeExists) && !errors.Is(err, zk.ErrClosing) {
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
