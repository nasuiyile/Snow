package broadcast

import (
	"bytes"
	"encoding/gob"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net"
	. "snow/common"
	. "snow/config"
	"snow/internal/membership"
	"snow/internal/state"
	"snow/tool"
	"sync/atomic"
	"time"
)

// Server 定义服务器结构体
type Server struct {
	listener         net.Listener
	Config           *Config
	Member           membership.MemberShipList
	State            state.State
	Action           Action
	client           net.Dialer  //客户端连接器
	IsClosed         atomic.Bool //是否关闭了
	H                HandlerFunc
	StopCh           chan struct{}
	SendChan         chan *SendData
	HeartbeatService *Heartbeat // 心跳服务
	UdpServer        *UDPServer // UDP服务器
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
		if r.Action != nil {
			go (*r.Action)(true)
		}
		if r.IsRoot {
			if configAction != nil {
				go (*configAction)(true)
			}
			return
		}

		newMsg := make([]byte, len(msg))
		copy(newMsg, msg)
		copy(newMsg[prefix:prefix+IpLen], s.Config.IPBytes())
		s.SendMessage(tool.ByteToIPv4Port(r.Ip), []byte{}, newMsg)
		//断开连接
		if msgAction == NodeLeave {
			//s.Member.RemoveMember(msg[len(msg)-IpLen:], false)
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
	return s.State.State.Add(m, "", s.Config.ExpirationTime)
}

func ObtainOnIPRing(current int, offset int, n int) int {
	return (current + offset + n) % n
}

// InitMessage 发消息
func (s *Server) InitMessage(msgType MsgType, action MsgAction) (map[string][]byte, []byte) {
	s.Member.Lock()
	defer s.Member.Unlock()
	if s.Member.MemberLen() == 1 {
		return make(map[string][]byte), nil
	}
	current := s.Member.Find(s.Config.IPBytes())
	//当前的索引往左偏移
	leftIndex := ObtainOnIPRing(current, -(s.Member.MemberLen())/2, s.Member.MemberLen())
	//右索引是左索引-1，这是环的特性
	rightIndex := ObtainOnIPRing(leftIndex, -1, s.Member.MemberLen())
	leftIP := s.Member.IPTable[leftIndex]
	rightIP := s.Member.IPTable[rightIndex]
	return s.NextHopMember(msgType, action, leftIP, rightIP, true)
}

func (s *Server) NextHopMember(msgType MsgType, msgAction MsgAction, leftIP []byte, rightIP []byte, isRoot bool) (map[string][]byte, []byte) {
	s.Member.Lock()
	def := func() { s.Member.Unlock() }

	coloring := msgType == ColoringMsg
	if s.Member.MemberLen() == 1 {
		def()
		return make(map[string][]byte), nil
	}
	forwardList := make(map[string][]byte)
	//要转发的所有节点
	leftIndex, lok := s.Member.FindOrInsert(leftIP)
	rightIndex, rok := s.Member.FindOrInsert(rightIP)
	//如果更新了就重新找一遍节点
	if lok || rok {
		leftIndex = s.Member.Find(leftIP)
		rightIndex = s.Member.Find(rightIP)
		//引用也要重新更新,这些元素是被临时插入的用完就需要删除
		if lok && rok {
			def = func() {
				s.Member.IPTable = tool.DeleteAtIndexes(s.Member.IPTable, leftIndex, rightIndex)
				s.Member.Unlock()
			}
		} else if lok {
			def = func() {
				s.Member.IPTable = tool.DeleteAtIndexes(s.Member.IPTable, leftIndex)
				s.Member.Unlock()
			}
		} else if rok {
			def = func() {
				s.Member.IPTable = tool.DeleteAtIndexes(s.Member.IPTable, rightIndex)
				s.Member.Unlock()
			}
		}
	}
	currentIndex := s.Member.Find(s.Config.IPBytes())

	k := s.Config.FanOut

	next, areaLen := CreateSubTree(leftIndex, rightIndex, currentIndex, s.Member.MemberLen(), k, coloring)
	//构建 secondary tree,注意这里的左边界和右边界要和根节点保持一致
	if isRoot && areaLen > (1+k) && coloring {
		secondaryRoot := ObtainOnIPRing(currentIndex, 1, s.Member.MemberLen())
		next = append(next, &area{left: leftIndex, right: rightIndex, current: secondaryRoot})
	}
	randomNumber := tool.RandomNumber()
	for _, v := range next {
		payload := make([]byte, 0)
		payload = append(payload, msgType)
		payload = append(payload, msgAction)
		payload = append(payload, s.Member.IPTable[v.left]...)
		payload = append(payload, s.Member.IPTable[v.right]...)
		if isRoot {
			payload = append(payload, randomNumber...)
		}
		forwardList[tool.ByteToIPv4Port(s.Member.IPTable[v.current])] = payload

	}
	def()

	return forwardList, randomNumber
}

// 加入的时候要和一个节点进行交互，离开则不用
func (s *Server) ApplyJoin(ip string) {
	if ip == s.Config.ServerAddress {
		return
	}
	s.SendMessage(ip, []byte{}, tool.PackTag(NodeChange, ApplyJoin))
}

func (s *Server) ApplyLeave() {
	f := func(isSuccess bool) {
		//如果成功了，当前节点下线。如果不成功，在发起一次请求
		if isSuccess {
			time.Sleep(5 * time.Second)
			//进行下线操作
			stop := struct{}{}
			s.StopCh <- stop
			s.Close()
			s.Member.Clean()
			s.IsClosed.Store(true)
		} else {
			//失败就再发一次
			s.ApplyLeave()
		}
	}
	s.ReliableMessage(s.Config.IPBytes(), NodeLeave, &f)
}
func (s *Server) ReportLeave(ip []byte) {
	if tool.ByteToIPv4Port(ip) != "127.0.0.1:40026" && s.Config.ServerAddress != "127.0.0.1:40026" {
		fmt.Println(tool.ByteToIPv4Port(ip))
	}
	log.Warnf(s.Config.ServerAddress + "report leave:" + tool.ByteToIPv4Port(ip))
	s.Member.RemoveMember(ip, false)
	s.ColoringMessage(ip, ReportLeave)
}

func (s *Server) exportState() []byte {
	s.Member.Lock()
	defer s.Member.Unlock()
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
	s.Member.InitState(MetaData)
}
