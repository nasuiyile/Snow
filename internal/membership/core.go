package membership

import (
	"net"
	"snow/tool"
	"sort"
	"time"
)

type MetaData struct {
	client  net.Conn
	server  net.Conn
	Version int
}

type MemberShipList struct {
	tool.ReentrantLock // 保护 clients 的并发访问并保证 并集群成员有相同的视图
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

func (m *MemberShipList) InitState(metaDataMap map[string]*MetaData, currentIp []byte) {
	m.Lock()
	defer m.Unlock()
	keys := make([]string, 0, len(metaDataMap)) // 预分配容量以优化性能
	for k := range metaDataMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for k, v := range metaDataMap {
		node, ok := m.MetaData[k]
		if ok {
			if node.Version > v.Version {
				//所有的元数据都要写这里
				node.Version = v.Version
			}
		} else {
			m.MetaData[k] = v
		}
		m.FindOrInsert(tool.IPv4To6Bytes(k))
	}
}

func (m *MetaData) GetServer() net.Conn {
	return m.server
}

func (m *MetaData) SetServer(server net.Conn) {
	m.server = server
}

func (m *MetaData) GetClient() net.Conn {
	return m.client
}

func (m *MetaData) SetClient(client net.Conn) {
	m.client = client
}

func (m *MemberShipList) MemberLen() int {
	return len(m.IPTable)
}

// FindOrInsert 第二个参数表示是否进行了更新,每次调用这个方法索引就会刷新
func (m *MemberShipList) FindOrInsert(target []byte) (int, bool) {
	m.Lock()
	defer m.Unlock()
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
	copy(m.IPTable[index+1:], m.IPTable[index:])
	m.IPTable[index] = append([]byte{}, target...) // 插入新元素

	//fmt.Println("Inserted at index:", index)

	return index, true

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
		Version: 0,
		client:  nil,
	}
}

// 和addNode的区别是不需要实际进行连接
func (m *MemberShipList) AddMember(ip []byte) {
	m.Lock()
	defer m.Unlock()
	_, ok := m.MetaData[tool.ByteToIPv4Port(ip)]
	if !ok {
		m.MetaData[tool.ByteToIPv4Port(ip)] = NewEmptyMetaData()
	}
	m.FindOrInsert(ip)
}
func (m *MemberShipList) RemoveMember(ip []byte, close bool) {
	m.Lock()
	defer m.Unlock()
	address := tool.ByteToIPv4Port(ip)
	data, ok := m.MetaData[address]
	if ok && close {
		if data.client != nil {
			time.AfterFunc(3*time.Second, func() {
				tcpConn := (data.client).(*net.TCPConn)
				tcpConn.SetLinger(0)
				tcpConn.Close()
			})
		}
		if data.server != nil {
			time.AfterFunc(3*time.Second, func() {
				tcpConn := (data.server).(*net.TCPConn)
				tcpConn.SetLinger(0)
				tcpConn.Close()
			})
		}
		//扇出后还是可能短暂的发送消息
		delete(m.MetaData, tool.ByteToIPv4Port(ip))
	}
	idx, _ := m.FindOrInsert(ip)
	//删除当前元素
	m.IPTable = append(m.IPTable[:idx], m.IPTable[idx+1:]...)
}
func (m *MemberShipList) GetMember(key string) *MetaData {
	m.Lock()
	defer m.Unlock()
	return m.MetaData[key]
}
func (m *MemberShipList) PutMemberIfNil(key string, value *MetaData) {
	m.Lock()
	defer m.Unlock()
	data := m.MetaData[key]
	if data == nil {
		m.MetaData[key] = value
		return
	}
	if data.client == nil && value.client != nil {
		data.client = value.client
	}
	if data.server == nil && value.server != nil {
		data.server = value.server
	}
	//m.FindOrInsert(tool.IPv4To6Bytes(key))
}
