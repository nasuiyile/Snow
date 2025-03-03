package main

import (
	"fmt"
	"net"
	"snow/tool"
	"sort"
)

// Server 定义服务器结构体
type Server struct {
	listener net.Listener
	//clients  map[net.Conn]bool // 存储所有客户端连接
	Config Config
	Member MemberShipList
}

type MemberShipList struct {
	lock tool.ReentrantLock // 保护 clients 的并发访问并保证 并集群成员有相同的视图
	//这个就是membership list
	IPTable [][]byte
	//这里存放连接和元数据
	MetaData map[string]MetaData
}

type MetaData struct {
	clients net.Conn
	version int
}
type area struct {
	current int
	left    int
	right   int
}

func (m *MemberShipList) MemberLen() int {
	return len(m.IPTable)
}

// 第二个参数表示是否进行了更新,每次调用这个方法索引就会刷新
func (m *MemberShipList) FindOrInsert(target []byte) (int, bool) {
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

// 比较两个 []byte 的大小
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

func NewMetaData(conn net.Conn) MetaData {
	return MetaData{
		version: 0,
		clients: conn,
	}
}
func (m *MemberShipList) AddNode(conn net.Conn, joinRing bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
	fmt.Println(conn.LocalAddr().String())
	addr := conn.RemoteAddr().String()
	v, ok := m.MetaData[addr]
	if ok {
		v.clients = conn
	} else {
		m.MetaData[addr] = NewMetaData(conn)
	}
	if joinRing {
		bytes := IPv4To6Bytes(addr)
		m.FindOrInsert(bytes)
	}

}

func ObtainOnIPRing(current int, offset int, n int) int {
	return (current + offset + n) % n
}

// 发消息
func (s *Server) InitMessage() map[string][]byte {
	s.Member.lock.Lock()
	defer s.Member.lock.Unlock()
	if s.Member.MemberLen() == 0 {
		return make(map[string][]byte)
	}
	current, _ := s.Member.FindOrInsert(s.Config.IPBytes())
	//当前的索引往左偏移
	leftIndex := ObtainOnIPRing(current, -(s.Member.MemberLen()-1)/2, s.Member.MemberLen())
	//右索引是左索引-1，这是环的特性
	rightIndex := ObtainOnIPRing(leftIndex, -1, s.Member.MemberLen())
	leftIP := s.Member.IPTable[leftIndex]
	rightIP := s.Member.IPTable[rightIndex]
	// 6
	return s.NextHopMember(userMsg, leftIP, rightIP)
}

func (s *Server) NextHopMember(header byte, leftIP []byte, rightIP []byte) map[string][]byte {
	//todo 这里可以优化读写锁
	s.Member.lock.Lock()
	defer s.Member.lock.Unlock()
	forwardList := make(map[string][]byte)

	//要转发的所有节点
	IPTable := s.Member.IPTable
	leftIndex, lok := s.Member.FindOrInsert(leftIP)
	rightIndex, rok := s.Member.FindOrInsert(rightIP)
	currentIndex, cok := s.Member.FindOrInsert(s.Config.IPBytes())
	//如果更新了就重新找一遍节点
	if lok || rok || cok {
		leftIndex, _ = s.Member.FindOrInsert(leftIP)
		rightIndex, _ = s.Member.FindOrInsert(rightIP)
		currentIndex, _ = s.Member.FindOrInsert(s.Config.IPBytes())
	}
	//构建子树
	next := createSubTree(leftIndex, rightIndex, currentIndex, s.Member.MemberLen(), s.Config.FanOut)
	for _, v := range next {
		payload := make([]byte, 0)
		payload = append(payload, header)
		payload = append(payload, IPTable[v.left]...)
		payload = append(payload, IPTable[v.right]...)
		forwardList[ByteToIPv4Port(IPTable[v.current])] = payload
	}
	return forwardList
}
func createSubTree(left int, right int, current int, n int, k int) []area {
	offset := 0
	//偏移到正数方便算
	if left > right {
		offset = left
		current = ObtainOnIPRing(right, -left, n)
		right = ObtainOnIPRing(right, -left, n)
		left = 0
	}
	AreaLen := right - left + 1

	areas := make([]area, 0)
	//除去自己的，剩余节点小于等于k，那就直接转发
	if (AreaLen - 1) <= k {
		for ; left <= right; left++ {
			areas = append(areas, area{current: left + offset, left: left + offset, right: left + offset})
		}
	}
	return areas
}
