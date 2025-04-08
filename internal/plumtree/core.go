package plumtree

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	. "snow/common"
	. "snow/config"
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
	isInitialized  atomic.Bool
	eagerLock      sync.RWMutex
	EagerPush      *tool.SafeSet[string]
	MessageIdQueue chan []byte
	msgCache       *state.TimeoutMap //缓存最近全部的消息
}

func NewServer(config *Config, action broadcast.Action) (*Server, error) {
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
		LazyPushTimeout:  6 * time.Second,
	}
	server.EagerPush = tool.NewSafeSet[string]()
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
			//已经收到消息了，发送PRUNE进行修剪
			sourceIp := tool.ByteToIPv4Port(msg[TagLen : TagLen+IpLen])
			//不对根节点进行修剪
			if sourceIp == parentIP {
				return
			}
			payload := tool.PackTag(Prune, msgAction)
			s.SendMessage(parentIP, payload, []byte{})
			return
		}
		//发送和收到时候都要进行缓存
		msgId := msg[TagLen+IpLen : IpLen+TagLen+TimeLen]
		s.msgCache.Set(string(msgId), string(msg), s.Config.ExpirationTime)
		s.MessageIdQueue <- msgId
		ipByte := msg[TagLen : TagLen+IpLen]
		switch msgAction {
		case NodeJoin:
			s.eagerLock.Lock()
			s.Member.AddMember(ipByte, NodeSurvival)
			sourceIp := tool.ByteToIPv4Port(ipByte)
			//不等于自己
			if !bytes.Equal(ipByte, s.Config.IPBytes()) {
				//port := tool.GetPortByIp(sourceIp)
				//if tool.IsLastDigitEqual(s.Config.Port, port) {
				s.EagerPush.Add(sourceIp)
				//}
			}
			s.eagerLock.Unlock()
			return
		case NodeLeave:
			s.eagerLock.Lock()
			ip := msg[TagLen+IpLen+TimeLen : TagLen+TimeLen+IpLen*2]
			s.Member.RemoveMember(ip, false)
			s.EagerPush.Remove(tool.ByteToIPv4Port(ip))
			s.eagerLock.Unlock()
		}
		//对消息进行转发
		s.PlumTreeMessage(msg)
	case Prune:
		//从eagerPush里进行删除
		s.EagerPush.Remove(parentIP)
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
		s.EagerPush.Add(parentIP)
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
	msgId := bytes[TagLen+IpLen : TagLen+IpLen+TimeLen]
	s.msgCache.Add(msgId, string(msg), s.Config.ExpirationTime)
	s.PlumTreeMessage(bytes)
}
func (s *Server) PlumTreeMessage(msg []byte) {
	if !s.isInitialized.Load() {
		s.eagerLock.Lock()
		//如果树还没初始化过就先进行初始化，初始化的f个节点直接使用扇出来做
		nodes := s.Server.KRandomNodes(s.Server.Config.FanOut)
		s.EagerPush = tool.NewSafeSetFromSlice(nodes)
		s.isInitialized.Store(true)
		s.eagerLock.Unlock()
	}
	s.EagerPush.Range(func(key string) bool {
		s.SendMessage(key, []byte{}, msg)
		return true
	})
}

func (s *Server) SendMessage(ip string, payload []byte, msg []byte) {
	if s.IsClosed {
		return
	}
	go func() {
		metaData := s.Member.GetMember(ip)
		var conn net.Conn
		var err error
		if metaData == nil {
			conn, err = s.ConnectToPeer(ip)
			if err != nil {
				log.Println(s.Config.ServerAddress, "can't connect to ", ip)
				s.EagerPush.Remove(ip)
				s.ReportLeave(tool.IPv4To6Bytes(ip))
				log.Println(err)
				return
			}
		} else {
			conn = metaData.GetClient()
		}
		if conn == nil {
			//先建立一次链接进行尝试
			newConn, err := s.ConnectToPeer(ip)
			if err != nil {
				log.Println(s.Config.ServerAddress, "can't connect to ", ip)
				s.EagerPush.Remove(ip)
				s.ReportLeave(tool.IPv4To6Bytes(ip))
				return
			} else {
				conn = newConn
			}
		}
		// 创建消息头，存储消息长度 (4字节大端序)
		length := uint32(len(payload) + len(msg))

		header := make([]byte, 4)
		binary.BigEndian.PutUint32(header, length)

		data := &broadcast.SendData{
			Conn:    conn,
			Header:  header,
			Payload: payload,
			Msg:     msg,
		}
		s.SendChan <- data
	}()

}
