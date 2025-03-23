package broadcast

import (
	"bytes"
	"net"
	. "snow/common"
	"snow/tool"
)

func (s *Server) Hand(msg []byte, conn net.Conn) {
	parentIP := s.Config.GetServerIp(conn.RemoteAddr().String())
	//判断消息类型
	msgType := msg[0]
	//消息要进行的动作
	msgAction := msg[1]
	NodeChanging(msg[1:], parentIP, s, conn)

	switch msgType {
	case RegularMsg:
		body := tool.CutBytes(msg)
		if !IsFirst(body, msgType, msgAction, s) {
			return
		}
		forward(msg, s, parentIP)
	case ColoringMsg:
		body := tool.CutBytes(msg)

		first := IsFirst(body, msgType, msgAction, s)
		forward(msg, s, parentIP)
		if !first {
			return
		}
		if msgAction == ReportLeave {
			s.Member.RemoveMember(tool.CutTimestamp(body), false)
		}
	case ReliableMsg:
		body := tool.CutBytes(msg)
		if !IsFirst(body, msgType, msgAction, s) {
			return
		}
		if msgAction == NodeLeave {
			s.Member.RemoveMember(msg[len(msg)-IpLen:], false)
		}
		//如果自己是叶子节点发送ack给父节点	并删除ack的map
		forward(msg, s, parentIP)
	case ReliableMsgAck:
		//ack不需要ActionType
		body := msg[TagLen:]
		//去重的消息可能会过滤掉相同的ack。在消息尾部追加ip来解决
		if !IsFirst(body, msgType, msgAction, s) {
			return
		}

		//减少计数器
		s.ReduceReliableTimeout(msg, s.Action.ReliableCallback)
	case GossipMsg:
		//gossip不需要和Snow算法一样携带俩个ip
		body := msg[TagLen:]
		if !IsFirst(body, msgType, msgAction, s) {
			return
		}
		data := make([]byte, len(msg))
		copy(data, msg)
		s.SendGossip(data)
	case NodeChange:
		//分别是消息类型，消息时间戳，加入节点的ip
		if !IsFirst(msg[1:], msgType, msgAction, s) {
			return
		}

	}
}

func IsFirst(body []byte, msgType MsgType, action MsgAction, s *Server) bool {
	if s.IsReceived(body) && s.Config.ExpirationTime > 0 {
		return false
	}
	if action == UserMsg && msgType != ReliableMsgAck {
		//如果第二个byte的类型是userMsg才让用户进行处理
		body = tool.CutTimestamp(body)
		//这是让用户自己判断消息是否处理过
		if !s.Action.process(body) {
			return false
		}
	}
	return true
}

// 返回自己是不是转发成功，不成功说明是叶子节点
func forward(msg []byte, s *Server, parentIp string) {
	member := make(map[string][]byte)
	msgType := msg[0]
	msgAction := msg[1]
	leftIP := msg[TagLen : IpLen+TagLen]
	rightIP := msg[IpLen+TagLen : IpLen*2+TagLen]
	isLeaf := bytes.Compare(leftIP, rightIP) == 0

	if !isLeaf {
		member, _ = s.NextHopMember(msgType, msgAction, leftIP, rightIP, false)
	}
	//消息中会附带发送给自己的节点
	if msgType == ReliableMsg {
		//写入map 以便根据ack进行删除
		b := tool.CutBytes(msg)
		hash := []byte(tool.Hash(b))
		if len(member) == 0 {
			//叶子节点 直接发送ack
			//消息内容为2个type，加上当前地址长度+ack长度
			newMsg := make([]byte, 0)
			newMsg = append(newMsg, ReliableMsgAck)
			newMsg = append(newMsg, msgAction)
			newMsg = append(newMsg, tool.RandomNumber()...)
			newMsg = append(newMsg, hash...)
			// 叶子节点ip
			newMsg = append(newMsg, s.Config.IPBytes()...)
			//根节点ip
			newMsg = append(newMsg, msg[len(msg)-IpLen:]...)
			//if msgAction == NodeLeave {
			//	s.Member.RemoveMember(msg[len(msg)-IpLen:], false)
			//}

			s.SendMessage(parentIp, []byte{}, newMsg)
		} else {
			//不是发送节点的化，不需要任何回调
			s.State.AddReliableTimeout(hash, false, len(member), tool.IPv4To6Bytes(parentIp), nil)
		}
		for _, payload := range member {
			payload = append(payload, s.Config.IPBytes()...)
		}

	}

	s.ForwardMessage(msg, member)

}
