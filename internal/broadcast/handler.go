package broadcast

import (
	"bytes"
	"log"
	"net"
	"snow/tool"
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
	//以上的都是服务发现的
	coloringMsg
	regularMsg     //普通的消息
	reliableMsg    //可靠消息
	reliableMsgAck //可靠消息回执
	gossipMsg      //gossip消息
)

func handler(msg []byte, s *Server, conn net.Conn) {
	parentIP := conn.RemoteAddr().String()
	//s.Member.lock.Lock()
	//defer s.Member.lock.Unlock()
	//判断消息类型
	msgType := msg[0]
	switch msgType {
	case regularMsg:
		//转发逻辑和普通的用户消息是一样的
		body := s.Config.CutBytes(msg)
		if isFirst(body, s) {
			forward(msg, s, parentIP)
		}
	case coloringMsg:
		//转发逻辑和普通的用户消息是一样的
		body := s.Config.CutBytes(msg)
		if isFirst(body, s) {
			forward(msg, s, parentIP)
		}
	case reliableMsg:
		//转发逻辑和普通的用户消息是一样的
		body := s.Config.CutBytes(msg)
		if isFirst(body, s) {
			//如果自己是叶子节点发送ack给父节点	并删除ack的map
			forward(msg, s, parentIP)
		}
	case reliableMsgAck:
		body := msg[1:]
		//去重的消息可能会过滤掉相同的ack。在消息尾部追加ip来解决
		if isFirst(body, s) {
			//减少计数器
			if s.Action.ReliableCallback != nil {
				body = body[:len(body)-s.Config.IpLen()]
				s.ReduceReliableTimeout(body, *s.Action.ReliableCallback)
			}
		}
	default:
		log.Printf("Received non type message from %v: %s\n", conn.RemoteAddr(), string(msg))
	}
}

func isFirst(body []byte, s *Server) bool {
	if s.IsReceived(body) && s.Config.ExpirationTime > 0 {
		return false
	}
	body = s.Config.CutTimestamp(body)

	//这是让用户自己判断消息是否处理过
	if !s.Action.process(body) {
		return false
	}
	return true
}

// 返回自己是不是转发成功，不成功说明是叶子节点
func forward(msg []byte, s *Server, parentIp string) {
	member := make(map[string][]byte)
	msgType := msg[0]
	leftIP := msg[1 : s.Config.IpLen()+1]
	rightIP := msg[s.Config.IpLen()+1 : s.Config.IpLen()*2+1]
	isLeaf := bytes.Compare(leftIP, rightIP) == 0
	if !isLeaf {
		member, _ = s.NextHopMember(msgType, leftIP, rightIP, false)
	}
	//消息中会附带发送给自己的节点
	if msgType == reliableMsg {
		//写入map
		b := s.Config.CutBytes(msg)
		hash := []byte(tool.Hash(b))
		if len(member) == 0 {
			//叶子节点 直接发送ack
			//消息内容为1个type，加上当前地址长度+ack长度
			newMsg := make([]byte, 1+len(hash)+s.Config.IpLen())
			copy(newMsg[1+len(hash):], s.Config.IPBytes())
			newMsg[0] = reliableMsgAck
			copy(newMsg[1:], hash)
			s.SendMessage(parentIp, newMsg)
		} else {
			s.State.AddReliableTimeout(hash, false, len(member), IPv4To6Bytes(parentIp))
		}
		for _, payload := range member {
			payload = append(payload, s.Config.IPBytes()...)
		}

	}
	s.ForwardMessage(msg, member)

}
