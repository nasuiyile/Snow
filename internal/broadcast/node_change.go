package broadcast

import (
	"net"
	. "snow/common"
	"snow/util"
)

func initMsg(msg []byte) (MsgAction, int64, []byte) {
	//1 byte的类型，8byte的时间戳，然后是发消息节点的 ip加端口号
	time := util.BytesToTime(msg[1 : 1+TimeLen])
	return msg[0], time, msg[1+TimeLen:]
}

func NodeChanging(msg []byte, ip string, s *Server, conn net.Conn) {
	changeType, _, data := initMsg(msg)
	msg = msg[1:]
	switch changeType {
	case ApplyJoin:
		applyJoining(s, ip, conn)
	case JoinStateSync:
		joinStateSynchronizing(ip, data, s)
	case RegularStateSync:
		regularStateSynchronizing(msg, s)
	default:
	}
}
func applyJoining(s *Server, ip string, conn net.Conn) {
	//接收到消息然后推送
	state := s.exportState()
	data := util.PackTagToHead(NodeChange, JoinStateSync, state)
	s.SendMessage(ip, []byte{}, data)
	//s.replayMessage(conn, s.Config, data)
}

// 接收到同步消息
func joinStateSynchronizing(ip string, msg []byte, s *Server) {
	//同步节点的信息，同步完毕之后请求加入节点
	s.importState(msg)
	//使用标准消息广播自己需要加入
	s.RegularMessage(s.Config.IPBytes(), NodeJoin)
}

func regularStateSynchronizing(msg []byte, s *Server) {
	s.importState(msg[TimeLen:])
}
