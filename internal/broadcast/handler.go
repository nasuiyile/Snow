package broadcast

import (
	"bytes"
	"net"
	. "snow/common"
	"snow/util"
)

// Hand 处理从 TCP 连接收到的消息。逻辑与 UDP 类似，只是发送方式不同。
func (s *Server) Hand(msg []byte, conn net.Conn) {
	if s.IsClosed.Load() {
		return
	}
	parentIP := s.Config.GetServerIp(conn.RemoteAddr().String())
	// 假设 msg 的第 0 字节是消息类型，第 1 字节是动作类型
	msgType := msg[0]
	msgAction := msg[1]

	switch msgType {
	case StandardMsg:
		body := util.CutBytes(msg)
		if !IsFirst(body, msgType, msgAction, s) {
			return
		}
		forward(msg, s, parentIP)
	case ColoringMsg:
		body := util.CutBytes(msg)
		first := IsFirst(body, msgType, msgAction, s)
		if first {
			if msgAction == ReportLeave {
				leaveNode := util.CutTimestamp(body)
				s.Member.RemoveMember(leaveNode, false)
			}
		}
		forward(msg, s, parentIP)
	case ReliableMsg:
		body := util.CutBytes(msg)
		if !IsFirst(body, msgType, msgAction, s) {
			return
		}
		if msgAction == NodeLeave {
			s.Member.RemoveMember(msg[len(msg)-IpLen:], false)
		}
		forward(msg, s, parentIP)
	case ReliableMsgAck:
		body := msg[TagLen:]
		if !IsFirst(body, msgType, msgAction, s) {
			return
		}
		s.ReduceReliableTimeout(msg, s.Action.ReliableCallback)
	case GossipMsg:
		body := msg[TagLen:]
		if !IsFirst(body, msgType, msgAction, s) {
			return
		}
		data := make([]byte, len(msg))
		copy(data, msg)
		s.SendGossip(data)
	case NodeChange:
		if !IsFirst(msg[1:], msgType, msgAction, s) {
			return
		}
		NodeChanging(msg[1:], parentIP, s, conn)
	case UnicastMsg:
		body := msg[TagLen:]
		if msgAction == UserMsg {
			s.Action.process(body)
		}
	case PingMsg:
		if msgAction == DirectPing {
			//tcp的直接探测逻辑
		} else if msgAction == PingAck {
			//tcp的探测回复逻辑
		}
	}
}

func IsFirst(body []byte, msgType MsgType, action MsgAction, s *Server) bool {
	if s.IsReceived(body) && s.Config.ExpirationTime > 0 {
		return false
	}
	if action == UserMsg && msgType != ReliableMsgAck {
		//如果第二个byte的类型是userMsg才让用户进行处理
		body = util.CutTimestamp(body)
		//这是让用户自己判断消息是否处理过
		if !s.Action.process(body) {
			return false
		}
	}
	return true
}

func forward(msg []byte, s *Server, parentIp string) {
	member := make(map[string][]byte)
	msgType := msg[0]
	msgAction := msg[1]
	leftIP := msg[TagLen : IpLen+TagLen]
	rightIP := msg[IpLen+TagLen : IpLen*2+TagLen]
	isLeaf := bytes.Compare(leftIP, rightIP) == 0

	if !isLeaf {
		s.Member.Lock()
		member, _ = s.NextHopMember(msgType, msgAction, leftIP, rightIP, false)
		s.Member.Unlock()
	}
	//缓存member，判断和上一次计算的member是否相同

	//消息中会附带发送给自己的节点
	if msgType == ReliableMsg {
		//写入map 以便根据ack进行删除
		b := util.CutBytes(msg)
		hash := []byte(util.Hash(b))
		if len(member) == 0 {
			//叶子节点 直接发送ack
			//消息内容为2个type，加上当前地址长度+ack长度
			newMsg := make([]byte, 0)
			newMsg = append(newMsg, ReliableMsgAck)
			newMsg = append(newMsg, msgAction)
			newMsg = append(newMsg, util.RandomNumber()...)
			newMsg = append(newMsg, hash...)
			// 叶子节点ip
			newMsg = append(newMsg, s.Config.IPBytes()...)
			//根节点ip
			//newMsg = append(newMsg, msg[len(msg)-IpLen:]...)
			s.SendMessage(parentIp, []byte{}, newMsg)
		} else {
			//不是发送节点的化，不需要任何回调
			s.State.AddReliableTimeout(hash, false, len(member), util.IPv4To6Bytes(parentIp), nil)
		}
		for _, payload := range member {
			payload = append(payload, s.Config.IPBytes()...)
		}
	}
	s.ForwardMessage(msg, member)

}
