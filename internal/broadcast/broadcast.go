package broadcast

import (
	"encoding/binary"
	"snow/tool"
	"time"
)

func (s *Server) StandardMessage(message string, msgAction MsgAction) error {
	s.Member.Lock()
	defer s.Member.Unlock()
	member, _ := s.InitMessage(regularMsg, msgAction)
	messageBytes := []byte(message)
	for ip, payload := range member {
		length := uint32(len(messageBytes) + s.Config.Placeholder())
		newMsg := make([]byte, length)
		copy(newMsg, payload)
		copy(newMsg[s.Config.Placeholder():], messageBytes)
		s.SendMessage(ip, newMsg)
	}
	return nil
}

func (s *Server) ColoringMessage(message string, msgAction MsgAction) error {
	s.Member.Lock()
	defer s.Member.Unlock()
	member, _ := s.InitMessage(coloringMsg, msgAction)
	messageBytes := []byte(message)
	for ip, payload := range member {
		length := uint32(len(messageBytes) + s.Config.Placeholder())
		newMsg := make([]byte, length)
		copy(newMsg, payload)
		copy(newMsg[s.Config.Placeholder():], messageBytes)
		s.SendMessage(ip, newMsg)
	}
	return nil
}

// GossipMessage gossip协议是有可能广播给发送给自己的节点的= =
func (s *Server) GossipMessage(msg string, msgAction MsgAction) error {
	bytes := make([]byte, len(msg)+TimeLen+TagLen)
	bytes[0] = gossipMsg
	bytes[1] = msgAction
	copy(bytes[TagLen:], tool.TimeBytes())
	copy(bytes[TagLen+TimeLen:], msg)
	return s.SendGossip(bytes)
}

func (s *Server) SendGossip(msg []byte) error {
	idx, _ := s.Member.FindOrInsert(s.Config.IPBytes())
	randomNodes := tool.GetRandomExcluding(0, s.Member.MemberLen()-1, idx, s.Config.FanOut)
	for _, v := range randomNodes {
		s.SendMessage(tool.ByteToIPv4Port(s.Member.IPTable[v]), msg)
	}
	return nil
}

// ForwardMessage 转发消息
func (s *Server) ForwardMessage(msg []byte, member map[string][]byte) error {
	for ip, payload := range member {
		msgCopy := make([]byte, len(msg))
		copy(msgCopy, msg)
		copy(msgCopy, payload)
		s.SendMessage(ip, msgCopy)
	}
	return nil
}

func (s *Server) ReliableMessage(message string, msgAction MsgAction) error {
	s.Member.Lock()
	defer s.Member.Unlock()
	member, unix := s.InitMessage(reliableMsg, msgAction)
	timeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timeBytes, uint64(unix))
	timeBytes = append(timeBytes, message...)
	hash := []byte(tool.Hash(timeBytes))
	s.State.AddReliableTimeout(hash, true, len(member), nil)
	timeout := s.Config.GetReliableTimeOut()
	time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		reliableTimeout := s.State.GetReliableTimeout(hash)
		//如果超时还没被删除就手动调用
		if reliableTimeout != nil {
			if s.Action.ReliableCallback != nil {
				(*s.Action.ReliableCallback)(false)
			}
		}
	})
	messageBytes := []byte(message)
	for ip, payload := range member {
		length := uint32(len(messageBytes) + s.Config.Placeholder())
		newMsg := make([]byte, length)
		copy(newMsg, payload)
		copy(newMsg[s.Config.Placeholder():], messageBytes)
		s.SendMessage(ip, newMsg)
	}
	return nil
}
