package main

import (
	"fmt"
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
		fmt.Println("收到用户消息")
		body := s.Config.CutBytes(msg)
		fmt.Printf("Received message from %v: %s\n", conn.RemoteAddr(), string(body))
		msg = forward(msg, s)
	default:
		fmt.Println("没有被处理的消息！")
	}
}

func forward(msg []byte, s *Server) []byte {
	leftIP := msg[:s.Config.IpLen()]
	rightIP := msg[s.Config.IpLen()-1 : s.Config.IpLen()*2]
	s.NextHopMember(msg[0], leftIP, rightIP)
	return msg[:s.Config.IpLen()*2-1]
}
