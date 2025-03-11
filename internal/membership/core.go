package membership

import (
	"fmt"
	"net"
	"snow/tool"
	"sort"
)

type MetaData struct {
	client  net.Conn
	Version int
}

type MemberShipList struct {
	tool.ReentrantLock // 保护 clients 的并发访问并保证 并集群成员有相同的视图
	//这个就是membership list
	IPTable [][]byte
	//这里存放连接和元数据
	MetaData map[string]*MetaData
}

func (m *MemberShipList) InitState(metaDataMap map[string]*MetaData) {
	m.Lock()
	defer m.Unlock()
	keys := make([]string, 0, len(metaDataMap)) // 预分配容量以优化性能
	for k := range metaDataMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	m.MetaData = metaDataMap
	ipTable := make([][]byte, 0)
	for _, v := range keys {
		ipTable = append(ipTable, tool.IPv4To6Bytes(v))
	}
	m.IPTable = ipTable
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

	fmt.Println("Inserted at index:", index)
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

func NewMetaData(conn net.Conn) *MetaData {
	return &MetaData{
		Version: 0,
		client:  conn,
	}
}
func (m *MemberShipList) AddNode(conn net.Conn, joinRing bool) {
	m.Lock()
	defer m.Unlock()
	addr := conn.RemoteAddr().String()
	v, ok := m.MetaData[addr]
	if ok {
		v.client = conn
	} else {
		m.MetaData[addr] = NewMetaData(conn)
	}
	if joinRing {
		bytes := tool.IPv4To6Bytes(addr)
		m.FindOrInsert(bytes)
	}
}
