package main

import "fmt"

type MsgType byte

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

func handler(msg []byte) {
	//判断消息类型
	msgType := MsgType(msg[0])
	switch msgType {
	case userMsg:
		fmt.Println("收到用户消息")
	default:
		fmt.Println("没有被处理的消息！")
	}
}
