package plumtree

import (
	"fmt"
	"net"
	. "snow/common"
	"snow/internal/broadcast"
	"snow/internal/state"
	"snow/tool"
	"sync"
	"sync/atomic"
	"time"
)

type Server struct {
	*broadcast.Server
	PConfig        *PConfig
	eagerLock      sync.RWMutex
	isInitialized  atomic.Bool
	EagerPush      []string
	MessageIdQueue chan []byte
	msgCache       *state.TimeoutMap //缓存最近全部的消息
}

func NewServer(config *broadcast.Config, action broadcast.Action) (*Server, error) {
	oldServer, err := broadcast.NewServer(config, action)
	if err != nil {
		panic(err)
	}
	//扩展原来的handler方法
	server := &Server{
		Server:        oldServer,
		isInitialized: atomic.Bool{},
	}
	oldServer.H = server
	server.isInitialized.Store(false)
	server.PConfig = &PConfig{
		LazyPushInterval: 1 * time.Second,
		LazyPushTimeout:  4 * time.Second,
	}
	server.MessageIdQueue = make(chan []byte, 10000)
	server.msgCache = state.NewTimeoutMap()
	go server.lazyPushTask(server.Server.StopCh)
	return server, nil
}

func (s *Server) Hand(msg []byte, conn net.Conn) {
	parentIP := s.Config.GetServerIp(conn.RemoteAddr().String())
	//判断消息类型
	msgType := msg[0]
	//消息要进行的动作
	msgAction := msg[1]
	body := msg[TagLen:]
	switch msgType {
	case EagerPush:
		body = msg[TagLen+IpLen:]
		//用了原有的去重逻辑
		if !broadcast.IsFirst(body, msgType, msgAction, s.Server) {
			//发送PRUNE进行修剪,
			sourceIp := tool.ByteToIPv4Port(msg[TagLen : TagLen+IpLen])
			//不对根节点进行修剪
			if sourceIp == parentIP {
				return
			}
			payload := tool.PackTag(Prune, msgAction)
			s.SendMessage(parentIP, payload, []byte{})
		} else {
			//发送和收到时候都要进行缓存
			msgId := msg[TagLen+IpLen : IpLen+TagLen+TimeLen]
			s.msgCache.Set(string(msgId), string(msg), s.Config.ExpirationTime)
			s.MessageIdQueue <- msgId
			//对消息进行转发
			s.PlumTreeMessage(msg)
		}
	case Prune:
		//从eagerPush里进行删除
		s.eagerLock.Lock()
		s.EagerPush = removeString(s.EagerPush, parentIP)
		s.eagerLock.Unlock()
	case LazyPush:
		//判断有没有收到过
		hash := string(body)
		res := s.msgCache.Get(hash)
		if res != nil {
			return
			//多次收到这个值
		}
		//首次收到，等待超时之后发送Graft
		time.AfterFunc(s.PConfig.LazyPushTimeout, func() {
			res := s.msgCache.Get(hash)
			if res != nil {
				return
				//多次收到这个值
			}
			//主动拉取消息
			s.SendMessage(parentIP, []byte{Graft, IHAVE}, body)
		})
	case Graft:
		//要将最近收到的所有消息进行维护，然后以补发消息
		value := s.msgCache.Get(string(body))
		if value != nil {
			data := []byte(fmt.Sprintf("%v", value))
			s.SendMessage(parentIP, []byte{}, data)
		}
		//补发消息，论文中并没有明确说明如果扇出达到限制时的操作。小于k可以加入，当EagerPush大于k时没有明说(论文里的k其实是f)
		//if len(s.EagerPush) < s.Config.FanOut {
		s.eagerLock.Lock()
		//没有包含自身的情况下才能append
		if !findString(s.EagerPush, parentIP) {
			s.EagerPush = append(s.EagerPush, parentIP)
		}
		s.eagerLock.Unlock()
		//}
	default:
		//如果都没匹配到再走之前的逻辑
		s.Server.Hand(msg, conn)
	}
}

func (s *Server) PlumTreeBroadcast(msg []byte, msgAction MsgAction) {
	bytes := make([]byte, len(msg)+TimeLen+TagLen+IpLen)
	bytes[0] = EagerPush
	bytes[1] = msgAction
	copy(bytes[TagLen:], s.Config.IPBytes())

	copy(bytes[TagLen+IpLen:], tool.RandomNumber())
	copy(bytes[TagLen+TimeLen+IpLen:], msg)

	//用随机数当做消息id 发送之前进行缓存
	msgId := msg[TagLen+IpLen : TagLen+IpLen+TimeLen]
	s.msgCache.Add(msgId, string(msg), s.Config.ExpirationTime)
	s.PlumTreeMessage(bytes)
}
func (s *Server) PlumTreeMessage(msg []byte) {
	if !s.isInitialized.Load() {
		//如果树还没初始化过就先进行初始化，初始化的f个节点直接使用扇出来做

		nodes := s.Server.KRandomNodes(s.Server.Config.FanOut)
		s.eagerLock.Lock()
		s.EagerPush = nodes
		s.eagerLock.Unlock()
		s.isInitialized.Store(true)
	}
	s.eagerLock.RLock()
	for _, v := range s.EagerPush {
		s.Server.SendMessage(v, []byte{}, msg)
	}
	s.eagerLock.RUnlock()
}

// removeString 从切片中删除指定的字符串（只删除第一个匹配项）
func removeString(slice []string, target string) []string {
	for i, s := range slice {
		if s == target {
			return append(slice[:i], slice[i+1:]...) // 删除目标元素
		}
	}
	return slice // 如果未找到目标字符串，返回原切片
}

// removeString 从切片中删除指定的字符串（只删除第一个匹配项）
func findString(slice []string, target string) bool {
	for _, v := range slice {
		if v == target {
			return true
		}
	}
	return false // 如果未找到目标字符串，返回原切片
}
