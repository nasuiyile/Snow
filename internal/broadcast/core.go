package broadcast

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"net"
	"snow/internal/membership"
	"snow/internal/state"
	"snow/tool"
	"time"
)

// Server 定义服务器结构体
type Server struct {
	listener net.Listener
	Config   *Config
	Member   membership.MemberShipList
	State    state.State
	Action   Action
	client   net.Dialer //客户端连接器
}

type area struct {
	current int
	left    int
	right   int
}

// Action 接收到消息后执行的操作，不允许对原byte进行修改。不需要的操作可以不进行设置
type Action struct {
	//同步操作，等操作执行之后，消息才会被继续广播，返回false代表不用继续广播了（用户自己做幂等操作的情况下才可以设置为false）
	SyncAction       *func([]byte) bool
	AsyncAction      *func([]byte)           //异步操作，不需要操作执行完，消息被处理的同时也可以被继续广播
	ReliableCallback *func(isConverged bool) //可靠消息的回调逻辑，只对根节点有作用。表示这条消息是否已经被广播到了全局
}

func (s *Server) ReduceReliableTimeout(msg []byte, configAction *func(isConverged bool)) {
	s.State.ReliableMsgLock.Lock()
	defer s.State.ReliableMsgLock.Unlock()
	//msgType := msg[0]
	//消息要进行的动作
	msgAction := msg[1]
	//截取32位
	prefix := TagLen + TimeLen + HashLen
	hash := string(msg[TagLen+TimeLen : prefix])
	r, ok := s.State.ReliableTimeout[hash]
	if !ok {
		return
	}
	r.Counter--

	if r.Counter == 0 {
		delete(s.State.ReliableTimeout, hash)
		//如果计数器为0代表已经收到了全部消息，这时候就可以触发根节点的回调方法
		if r.IsRoot {
			if configAction != nil {
				go (*configAction)(true)
			}
			return
		}
		if r.Action != nil {
			go (*r.Action)(true)
		}
		newMsg := make([]byte, len(msg))
		copy(newMsg, msg)
		copy(newMsg[prefix:prefix+s.Config.IpLen()], msg[prefix:prefix+s.Config.IpLen()])
		s.SendMessage(tool.ByteToIPv4Port(r.Ip), newMsg)
		//断开连接
		time.Sleep(3 * time.Second)
		if msgAction == nodeLeave {
			s.Member.RemoveMember(msg[:s.Config.IpLen()])
		}
	}

}

func (a *Action) process(body []byte) bool {
	if a.AsyncAction != nil {
		go (*a.AsyncAction)(body)
	}
	if a.SyncAction != nil {
		return (*a.SyncAction)(body)
	}
	return true
}

func (s *Server) IsReceived(m []byte) bool {
	return s.State.State.Add(m, s.Config.ExpirationTime)
}

func ObtainOnIPRing(current int, offset int, n int) int {
	return (current + offset + n) % n
}

// InitMessage 发消息
func (s *Server) InitMessage(msgType MsgType, action MsgAction) (map[string][]byte, int64) {
	s.Member.Lock()
	defer s.Member.Unlock()
	if s.Member.MemberLen() == 1 {
		return make(map[string][]byte), 0
	}
	current, _ := s.Member.FindOrInsert(s.Config.IPBytes())
	//当前的索引往左偏移
	leftIndex := ObtainOnIPRing(current, -(s.Member.MemberLen())/2, s.Member.MemberLen())
	//右索引是左索引-1，这是环的特性
	rightIndex := ObtainOnIPRing(leftIndex, -1, s.Member.MemberLen())
	leftIP := s.Member.IPTable[leftIndex]
	rightIP := s.Member.IPTable[rightIndex]

	return s.NextHopMember(msgType, action, leftIP, rightIP, true)
}

func (s *Server) NextHopMember(msgType MsgType, msgAction MsgAction, leftIP []byte, rightIP []byte, isRoot bool) (map[string][]byte, int64) {
	coloring := msgType == coloringMsg
	//todo 这里可以优化读写锁
	s.Member.Lock()
	defer s.Member.Unlock()
	if s.Member.MemberLen() == 1 {
		return make(map[string][]byte), 0
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
	next, areaLen := CreateSubTree(leftIndex, rightIndex, currentIndex, s.Member.MemberLen(), k, coloring)
	//构建 secondary tree,注意这里的左边界和右边界要和根节点保持一致
	if isRoot && areaLen > (1+k) && coloring {
		//next = make([]*area, 0)
		secondaryRoot := ObtainOnIPRing(currentIndex, -1, s.Member.MemberLen())
		next = append(next, &area{left: leftIndex, right: rightIndex, current: secondaryRoot})
	}
	unix := time.Now().Unix()
	for _, v := range next {
		payload := make([]byte, 0)
		payload = append(payload, msgType)
		payload = append(payload, msgAction)
		payload = append(payload, IPTable[v.left]...)
		payload = append(payload, IPTable[v.right]...)
		if isRoot {
			timestamp := make([]byte, 8)
			binary.BigEndian.PutUint64(timestamp, uint64(unix))
			payload = append(payload, timestamp...)
		}
		forwardList[tool.ByteToIPv4Port(IPTable[v.current])] = payload

	}

	return forwardList, unix
}

// 加入的时候要和一个节点进行交互，离开则不用
func (s *Server) ApplyJoin(ip string) {
	err := s.connectToClient(ip)
	if err != nil {
		return
	}
	s.SendMessage(ip, PackTag(nodeChange, applyJoin))
}

func (s *Server) ApplyLeave() {
	f := func(isSuccess bool) {
		//如果成功了，当前节点下线。如果不成功，在发起一次请求
		if isSuccess {
			//进行下线操作
			s.Close()
			s.Member.Clean()
		} else {
			//失败就再发一次
			s.ApplyLeave()
		}
	}
	s.ReliableMessage(s.Config.IPBytes(), nodeLeave, &f)
}
func (s *Server) ReportLeave(ip []byte) {
	s.ColoringMessage(ip, reportLeave)
}

func (s *Server) exportState() []byte {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(s.Member.MetaData)
	if err != nil {
		fmt.Println("GOB Serialization failed:", err)
		return nil
	}
	return buffer.Bytes()
}
func (s *Server) importState(msg []byte) {
	buffer := bytes.NewBuffer(msg)
	var MetaData map[string]*membership.MetaData
	decoder := gob.NewDecoder(buffer)
	err := decoder.Decode(&MetaData)
	if err != nil {
		fmt.Println("GOB Desialization failed:", err)
		return
	}
	s.Member.InitState(MetaData, s.Config.IPBytes())
}
