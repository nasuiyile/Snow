package broadcast

import (
	"encoding/binary"
	"snow/tool"
	"time"
)

func (s *Server) RegularMessage(message []byte, msgAction MsgAction) error {
	s.Member.Lock()
	defer s.Member.Unlock()
	member, _ := s.InitMessage(regularMsg, msgAction)
	for ip, payload := range member {
		length := uint32(len(message) + s.Config.Placeholder())
		newMsg := make([]byte, length)
		copy(newMsg, payload)
		copy(newMsg[s.Config.Placeholder():], message)
		s.SendMessage(ip, newMsg)
	}
	return nil
}

func (s *Server) ColoringMessage(message []byte, msgAction MsgAction) error {
	s.Member.Lock()
	defer s.Member.Unlock()
	member, _ := s.InitMessage(coloringMsg, msgAction)
	for ip, payload := range member {
		length := uint32(len(message) + s.Config.Placeholder())
		newMsg := make([]byte, length)
		copy(newMsg, payload)
		copy(newMsg[s.Config.Placeholder():], message)
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

// 在成功时，每个节点都会回调这个方法，在失败时只有根节点和部分成功的节点会重新调用这个方法
func (s *Server) ReliableMessage(message []byte, msgAction MsgAction, action *func(isSuccess bool)) error {
	s.Member.Lock()
	defer s.Member.Unlock()

	member, unix := s.InitMessage(reliableMsg, msgAction)
	timeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timeBytes, uint64(unix))
	timeBytes = append(timeBytes, message...)
	hash := []byte(tool.Hash(timeBytes))
	s.State.AddReliableTimeout(hash, true, len(member), nil, action)
	timeout := s.Config.GetReliableTimeOut()
	time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		reliableTimeout := s.State.GetReliableTimeout(hash)
		//如果超时还没被删除就手动调用
		if reliableTimeout != nil {
			if s.Action.ReliableCallback != nil {
				(*s.Action.ReliableCallback)(false)
			}
			if action != nil {
				(*action)(false)
			}
		}
	})
	for ip, payload := range member {
		length := uint32(len(message) + s.Config.Placeholder())
		newMsg := make([]byte, length)
		copy(newMsg, payload)
		copy(newMsg[s.Config.Placeholder():], message)
		s.SendMessage(ip, newMsg)
	}
	return nil
}
