package membership

import (
	"net"
	. "snow/common"
	"snow/util"
	"sort"
	"sync"
	"time"
)

type MetaData struct {
	sync.Mutex
	client     net.Conn
	server     net.Conn
	State      NodeState
	UpdateTime int64
	Version    int32 //现在的版本号没有实际的作用
}

type MemberShipList struct {
	sync.Mutex
	//tool.ReentrantLock // 保护 clients 的并发访问并保证 并集群成员有相同的视图
	//这个就是membership list
	IPTable [][]byte
	//这里存放连接和元数据
	MetaData map[string]*MetaData
}

func (m *MemberShipList) Clean() {
	m.Lock()
	defer m.Unlock()
	m.IPTable = make([][]byte, 0)
	m.MetaData = make(map[string]*MetaData)
}

func (m *MemberShipList) InitState(metaDataMap map[string]*MetaData) {
	m.Lock()
	defer m.Unlock()
	for k, v := range metaDataMap {
		if v.State == NodeLeft || v.State == NodePrepare {
			continue
		}
		node, ok := m.MetaData[k]
		if ok {
			if node.Version <= v.Version {
				//所有的元数据都要写这里
				node.Version = v.Version
				node.State = v.State
				node.UpdateTime = v.UpdateTime
			}
		} else {
			m.MetaData[k] = v
		}
		m.FindOrInsert(util.IPv4To6Bytes(k), false)
	}
}

func (m *MetaData) GetServer() net.Conn {
	return m.server
}

func (m *MetaData) SetServer(server net.Conn) {
	m.server = server
}

func (m *MetaData) GetClient(l bool) net.Conn {
	if l {
		m.Lock()
		defer m.Unlock()
	}
	return m.client
}

func (m *MetaData) SetClient(client net.Conn) {
	m.client = client
}

func (m *MemberShipList) MemberLen() int {
	return len(m.IPTable)
}

// FindOrInsert 第二个参数表示是否进行了更新,每次调用这个方法索引就会刷新
func (m *MemberShipList) FindOrInsert(target []byte, l bool) (int, bool) {
	if l {
		m.Lock()
		defer m.Unlock()
	}
	// 使用二分查找定位目标位置
	index := sort.Search(len(m.IPTable), func(i int) bool {
		return BytesCompare(m.IPTable[i], target) >= 0
	})

	// 如果找到相等的元素，直接返回原数组和索引
	if index < len(m.IPTable) && BytesCompare(m.IPTable[index], target) == 0 {
		return index, false
	}

	// 如果没有找到，插入到正确的位置
	m.IPTable = append(m.IPTable, nil)
	if index < len(m.IPTable)-1 {
		copy(m.IPTable[index+1:], m.IPTable[index:])
	}
	m.IPTable[index] = target // 假设可以安全引用外部target

	return index, true
}

func (m *MemberShipList) Find(target []byte, l bool) int {
	if l {
		m.Lock()
		defer m.Unlock()
	}
	// 使用二分查找定位目标位置
	index := sort.Search(len(m.IPTable), func(i int) bool {
		return BytesCompare(m.IPTable[i], target) >= 0
	})

	// 如果找到相等的元素，直接返回原数组和索引
	if index < len(m.IPTable) && BytesCompare(m.IPTable[index], target) == 0 {
		return index
	}
	return -1
}

// BytesCompare 比较两个 []byte 的大小
func BytesCompare(a, b []byte) int {
	if len(a) < len(b) {
		return -1
	} else if len(a) > len(b) {
		return 1
	}
	for i := range a {
		if a[i] < b[i] {
			return -1
		} else if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func NewEmptyMetaData() *MetaData {
	return &MetaData{
		Version:    0,
		UpdateTime: time.Now().Unix(),
		client:     nil,
		server:     nil,
		State:      NodePrepare,
	}
}

// 和addNode的区别是不需要实际进行连接
func (m *MemberShipList) AddMember(ip []byte, state NodeState) {

	metaData, ok := m.MetaData[util.ByteToIPv4Port(ip)]
	if !ok {
		metadata := NewEmptyMetaData()
		metadata.State = state
		m.MetaData[util.ByteToIPv4Port(ip)] = metadata
	} else {
		metaData.UpdateTime = time.Now().Unix()
	}
	m.FindOrInsert(ip, false)
}
func (m *MemberShipList) RemoveMember(ip []byte, close bool) {
	address := util.ByteToIPv4Port(ip)
	m.Lock()
	defer m.Unlock()
	idx := m.Find(ip, false)
	if idx == -1 {
		return
	}
	data, ok := m.MetaData[address]
	if ok {
		if close {
			if data.client != nil {
				time.AfterFunc(3*time.Second, func() {
					if tcpConn, ok := data.client.(*net.TCPConn); ok {
						// 设置SO_LINGER为0，立即关闭连接并发送RST而不是FIN
						tcpConn.SetLinger(0)
						_ = tcpConn.CloseRead()
						_ = tcpConn.CloseWrite()
					}
					_ = data.client.Close()
				})
			}
			if data.server != nil {
				time.AfterFunc(3*time.Second, func() {
					if tcpConn, ok := data.server.(*net.TCPConn); ok {
						// 设置SO_LINGER为0，立即关闭连接并发送RST而不是FIN
						tcpConn.SetLinger(0)
						_ = tcpConn.CloseRead()
						_ = tcpConn.CloseWrite()
					}
					_ = data.server.Close()
				})
			}
			//扇出后还是可能短暂的发送消息
			delete(m.MetaData, util.ByteToIPv4Port(ip))
		}
		data.UpdateTime = time.Now().Unix()
		data.State = NodeLeft
	}
	//找到就删除当前元素
	m.IPTable = util.DeleteAtIndexes(m.IPTable, idx)

}
func (m *MemberShipList) GetOrPutMember(key string) *MetaData {
	m.Lock()
	defer m.Unlock()
	var v *MetaData
	//缩小锁的范围
	value, ok := m.MetaData[key]
	if !ok {
		v = NewEmptyMetaData()
		m.MetaData[key] = v
	} else {
		v = value
	}
	return v
}
func (m *MemberShipList) PutMemberIfNil(key string, value *MetaData) {
	m.Lock()
	defer m.Unlock()
	data := m.MetaData[key]
	if data == nil {
		m.MetaData[key] = value
		return
	}
	if value.client != nil {
		data.client = value.client
	}
	if value.server != nil {
		data.server = value.server
	}
	//m.FindOrInsert(tool.IPv4To6Bytes(key))
}
