package plumtree

import (
	"net"
	. "snow/common"
	"snow/internal/broadcast"
	"snow/tool"
	"sync"
)

type Server struct {
	*broadcast.Server
	eagerLock sync.Mutex
	lazyLock  sync.Mutex

	eagerPush []string
	lazyPush  []string
}

func NewServer(config *broadcast.Config, action broadcast.Action) (*Server, error) {
	oldServer, err := broadcast.NewServer(config, action)
	if err != nil {
		panic(err)
	}
	//扩展原来的handler方法
	server := &Server{
		Server: oldServer,
	}
	oldServer.H = server

	return server, nil
}

func (s *Server) Hand(msg []byte, conn net.Conn) {
	parentIP := s.Config.GetServerIp(conn.RemoteAddr().String())
	//判断消息类型
	msgType := msg[0]
	//消息要进行的动作
	msgAction := msg[1]
	switch msgType {
	case EagerPush:
		body := msg[TagLen:]
		if !broadcast.IsFirst(body, msgType, msgAction, s.Server) {
			//发送PRUNE进行修剪
			payload := tool.PackTag(Prune, msgAction)
			s.SendMessage(parentIP, payload, []byte{})
		} else {
			//对消息进行转发
			s.PlumTreeBroadcast(msg, msgAction)
		}
	case Prune:
		//从eagerPush里进行删除
		s.eagerLock.Lock()
		s.eagerPush = removeString(s.eagerPush, parentIP)
		s.eagerLock.Unlock()
	default:
		//如果都没匹配到再走之前的逻辑
		s.Server.Hand(msg, conn)
	}
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
func (s *Server) PlumTreeBroadcast(msg []byte, action MsgAction) {
	if len(s.eagerPush) == 0 {
		//如果树还没初始化过就先进行初始化，初始化的f个节点直接使用扇出来做
		nodes := s.Server.KRandomNodes(s.Server.Config.FanOut)
		s.eagerLock.Lock()
		s.eagerPush = nodes
		s.eagerLock.Unlock()
	}
	for _, v := range s.eagerPush {
		payload := tool.PackTag(EagerPush, action)
		s.Server.SendMessage(v, payload, msg)
	}
}
