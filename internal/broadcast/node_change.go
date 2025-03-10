package broadcast

import (
	"snow/tool"
)

type ChangeType = byte

const (
	nodeJoin ChangeType = iota
	stateSync
	nodeLeave
)

func initMsg(msg []byte) (ChangeType, int64, []byte) {
	//1 byte的类型，8byte的时间戳，然后是发消息节点的 ip加端口号
	time := tool.BytesToTime(msg[1 : 1+TimeLen])
	return msg[0], time, msg[1+TimeLen:]
}

func NodeChange(pingMsg []byte, ip string, s *Server) {
	changeType, _, _ := initMsg(pingMsg)
	switch changeType {
	case nodeJoin:
		nodeJoining(ip, s)
	default:
	}
}
func nodeJoining(ip string, s *Server) {

}
