package broadcast

import (
	"log"
	"net"
)

type MsgType = byte

// 定义枚举值
const (
	pingMsg MsgType = iota
	indirectPingMsg
	ackRespMsg
	suspectMsg
	aliveMsg
	deadMsg
	userMsg
)

func handler(msg []byte, s *Server, conn net.Conn) {
	//判断消息类型
	msgType := msg[0]
	msg = msg[1:]
	switch msgType {
	case userMsg:
		// 打印接收到的消息
		log.Println(s.Config.LocalAddress, "get user message")
		body := s.Config.CutBytes(msg)
		log.Printf("Received message from %v: %s\n", conn.RemoteAddr(), string(body))
		msg = forward(msg, s)
	default:
		log.Printf("Received non type message from %v: %s\n", conn.RemoteAddr(), string(msg))
	}
}

func forward(msg []byte, s *Server) []byte {
	leftIP := msg[:s.Config.IpLen()]
	rightIP := msg[s.Config.IpLen() : s.Config.IpLen()*2]
	member := s.NextHopMember(msg[0], leftIP, rightIP)
	s.ForwardMessage(msg, member)
	return msg[:s.Config.IpLen()*2]
}
