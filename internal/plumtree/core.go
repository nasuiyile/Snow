package plumtree

import (
	"net"
	. "snow/common"
	"snow/internal/broadcast"
)

type Server struct {
	Server      *broadcast.Server
	eagerPushIp []string
	lazyPushIP  []string
}

func NewServer(config *broadcast.Config, action broadcast.Action) (*Server, error) {
	newServer, err := broadcast.NewServer(config, action)
	if err != nil {
		panic(err)
	}
	//扩展原来的handler方法
	newServer.Handler = Handler
	server := &Server{
		Server: newServer,
	}

	return server, nil
}
func (s *Server) PlumTreeBroadcast(msg []byte, action MsgAction) {
	if len(s.eagerPushIp) == 0 {
		//如果树还没初始化过就先进行初始化
	} else {
		for _, v := range s.eagerPushIp {
			payload := packageTag(EagerPush, action)
			s.Server.SendMessage(v, payload, msg)
		}
	}
}

func Handler(msg []byte, s *broadcast.Server, conn net.Conn) {
	//parentIP := s.Config.GetServerIp(conn.RemoteAddr().String())
	//判断消息类型
	msgType := msg[0]
	//消息要进行的动作
	//msgAction := msg[1]
	switch msgType {
	//case

	default:
		//如果都没匹配到再走之前的逻辑
		broadcast.Handler(msg, s, conn)
	}
}
func packageTag(msgType MsgType, action MsgAction) []byte {
	bytes := make([]byte, 2)
	bytes[0] = msgType
	bytes[1] = action
	return bytes
}
