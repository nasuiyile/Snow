package broadcast

import (
	"fmt"
	"log"
	"net"
	"time"
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
	//s.Member.lock.Lock()
	//defer s.Member.lock.Unlock()
	//判断消息类型
	msgType := msg[0]
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

// 解决问题 left right current
func forward(msg []byte, s *Server) []byte {
	time.Sleep(100 * time.Millisecond)
	isSame := true
	msgType := msg[0]
	leftIP := msg[1 : s.Config.IpLen()+1]
	rightIP := msg[s.Config.IpLen()+1 : s.Config.IpLen()*2+1]
	for i := 0; i < len(leftIP); i++ {
		if leftIP[i] != rightIP[i] {
			isSame = false
		}
	}
	if !isSame {
		member := s.NextHopMember(msgType, leftIP, rightIP)
		for ip, _ := range member {
			if ip == s.Config.LocalAddress {
				fmt.Println()
			}
		}
		s.ForwardMessage(msg, member)

	}

	return msg[:s.Config.Placeholder()]
}
