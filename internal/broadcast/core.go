package broadcast

import (
	"encoding/binary"
	"fmt"
	"net"
	"snow/internal/state"
	"snow/tool"
	"sort"
	"time"
)

// Server 定义服务器结构体
type Server struct {
	listener net.Listener
	Config   *Config
	Member   MemberShipList
	State    *state.TimeoutMap
}

type MemberShipList struct {
	lock tool.ReentrantLock // 保护 clients 的并发访问并保证 并集群成员有相同的视图
	//这个就是membership list
	IPTable [][]byte
	//这里存放连接和元数据
	MetaData map[string]*MetaData
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

func (s *Server) IsReceived(m []byte) bool {
	return s.State.Add(m, s.Config.ExpirationTime)
}

func (m *MemberShipList) MemberLen() int {
	return len(m.IPTable)
}

// FindOrInsert 第二个参数表示是否进行了更新,每次调用这个方法索引就会刷新
func (m *MemberShipList) FindOrInsert(target []byte) (int, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
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
		version: 0,
		clients: conn,
	}
}
func (m *MemberShipList) AddNode(conn net.Conn, joinRing bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
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
func CurrentIsMid(left int, current int, right int, n int) bool {
	if left <= right {
		return current >= left && current <= right
	}
	return current >= left || current <= right
}

// InitMessage 发消息
func (s *Server) InitMessage(msgType MsgType) map[string][]byte {
	s.Member.lock.Lock()
	defer s.Member.lock.Unlock()
	if s.Member.MemberLen() == 1 {
		return make(map[string][]byte)
	}
	current, _ := s.Member.FindOrInsert(s.Config.IPBytes())
	//当前的索引往左偏移
	leftIndex := ObtainOnIPRing(current, -(s.Member.MemberLen())/2, s.Member.MemberLen())
	//右索引是左索引-1，这是环的特性
	rightIndex := ObtainOnIPRing(leftIndex, -1, s.Member.MemberLen())
	leftIP := s.Member.IPTable[leftIndex]
	rightIP := s.Member.IPTable[rightIndex]

	return s.NextHopMember(msgType, leftIP, rightIP, true)
}

func (s *Server) NextHopMember(msgType byte, leftIP []byte, rightIP []byte, isRoot bool) map[string][]byte {
	//todo 这里可以优化读写锁
	s.Member.lock.Lock()
	defer s.Member.lock.Unlock()
	if s.Member.MemberLen() == 1 {
		return make(map[string][]byte)
	}
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
		//引用也要重新更新
		IPTable = s.Member.IPTable
	}
	k := s.Config.FanOut
	//构建子树
	next, areaLen := CreateSubTree(leftIndex, rightIndex, currentIndex, s.Member.MemberLen(), k, s.Config.Coloring)
	//构建 secondary tree,注意这里的左边界和右边界要和根节点保持一致
	if isRoot && areaLen > (1+k) {
		//next = make([]*area, 0)
		secondaryRoot := ObtainOnIPRing(currentIndex, -1, s.Member.MemberLen())
		next = append(next, &area{left: leftIndex, right: rightIndex, current: secondaryRoot})
	}
	for _, v := range next {
		payload := make([]byte, 0)
		payload = append(payload, msgType)
		payload = append(payload, IPTable[v.left]...)
		payload = append(payload, IPTable[v.right]...)
		timestamp := make([]byte, 8)
		binary.BigEndian.PutUint64(timestamp, uint64(time.Now().Unix()))
		payload = append(payload, timestamp...)
		forwardList[ByteToIPv4Port(IPTable[v.current])] = payload
	}

	return forwardList
}
func CreateSubTree(left int, right int, current int, n int, k int, coloring bool) ([]*area, int) {
	offset := 0
	//偏移到正数方便算
	if left > right {
		offset = left
		current = ObtainOnIPRing(current, -offset, n)
		right = ObtainOnIPRing(right, -offset, n)
		left = 0
	}
	tree := make([]*area, 0)
	//是否进行节点染色
	if coloring {
		tree = ColoringMultiwayTree(left, right, current, k)
	} else {
		tree = BalancedMultiwayTree(left, right, current, k)
	}
	areaLen := right - left + 1
	//计算完把偏移设置为原位
	for _, v := range tree {
		v.current = ObtainOnIPRing(v.current, offset, n)
		v.right = ObtainOnIPRing(v.right, offset, n)
		v.left = ObtainOnIPRing(v.left, offset, n)

	}

	return tree, areaLen
}

func BalancedMultiwayTree(left int, right int, current int, k int) []*area {

	AreaLen := right - left + 1
	areas := make([]*area, 0)
	//除去自己的，剩余节点小于等于k，那就直接转发
	if left > right {
		return nil
	} else if (AreaLen - 1) <= k {
		for ; left < right; left++ {
			if left == current {
				continue
			}
			//只有这一个节点了
			areas = append(areas, &area{
				current: left,
				left:    left,
				right:   left,
			})
		}
		return areas
	}
	leftArea := (current - left) / (k / 2)
	leftRemain := (current - left) % (k / 2)
	previousScope := left
	for i := 0; i < k/2; i++ {
		//将多余的区域从左边开始均分给每一个节点
		currentArea := leftArea
		if leftRemain > 0 {
			leftRemain--
			currentArea++
		}
		rightBound := previousScope + currentArea - 1
		leftNodeValue := (previousScope + (rightBound + 1)) / 2
		areas = append(areas, &area{left: previousScope, right: rightBound, current: leftNodeValue})
		previousScope = rightBound + 1
	}

	previousScope = current + 1
	rightArea := (right - current) / (k / 2)
	rightRemain := (right - current) % (k / 2)
	for i := 0; i < k/2; i++ {
		//将多余的区域从左边开始均分给每一个节点
		currentArea := rightArea
		if rightRemain > 0 {
			rightRemain--
			currentArea++
		}
		rightBound := previousScope + currentArea - 1
		rightNodeValue := (previousScope + (rightBound + 1)) / 2
		areas = append(areas, &area{left: previousScope, right: rightBound, current: rightNodeValue})

		previousScope = rightBound + 1
	}

	return areas
}

func ColoringMultiwayTree(left int, right int, current int, k int) []*area {
	AreaLen := right - left + 1
	areas := make([]*area, 0)
	//除去自己的，剩余节点小于等于k，那就直接转发
	if left > right {
		return nil
	} else if (AreaLen - 1) <= k {
		for ; left < current; left++ {
			areas = append(areas, &area{left: left, current: left, right: left})
		}
		for ; current < right; current++ {
			current++
			areas = append(areas, &area{left: current, current: current, right: current})
		}
		return areas
	}
	//当前找单数还是双数
	parity := current % 2
	leftArea := (current - left) / (k / 2)
	leftRemain := (current - left) % (k / 2)
	previousScope := left
	for i := 0; i < k/2; i++ {
		//将多余的区域从左边开始均分给每一个节点
		currentArea := leftArea
		if leftRemain > 0 {
			leftRemain--
			currentArea++
		}
		rightBound := previousScope + currentArea - 1
		leftNodeValue := (previousScope + (rightBound + 1)) / 2
		//先找到和当前单双数一样的节点,如果找不到的化就直接返回当前值
		if leftNodeValue%2 != parity {
			if leftNodeValue < rightBound {
				leftNodeValue++
			} else if leftNodeValue > previousScope {
				leftNodeValue--
			}
		}
		if leftNodeValue != current {
			areas = append(areas, &area{left: previousScope, right: rightBound, current: leftNodeValue})
		}
		previousScope = rightBound + 1
	}

	if previousScope >= right {
		return areas
	}
	previousScope = current + 1

	rightArea := (right - current) / (k / 2)
	rightRemain := (right - current) % (k / 2)
	for i := 0; i < k/2; i++ {

		//将多余的区域从左边开始均分给每一个节点
		currentArea := rightArea
		if rightRemain > 0 {
			rightRemain--
			currentArea++
		}
		rightBound := previousScope + currentArea - 1
		rightNodeValue := (previousScope + (rightBound + 1)) / 2
		//先找到和当前单双数一样的节点
		if rightNodeValue == 3 {
			fmt.Println()
		}
		if rightNodeValue%2 != parity {
			if rightNodeValue < rightBound {
				rightNodeValue++
			} else if rightNodeValue > previousScope {
				rightNodeValue--
			}
		}
		if rightNodeValue != current {
			areas = append(areas, &area{left: previousScope, right: rightBound, current: rightNodeValue})
		}
		previousScope = rightBound + 1
	}

	return areas

}
