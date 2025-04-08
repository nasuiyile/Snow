package broadcast

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	. "snow/common"
	"snow/tool"
	"time"
)

// Hand 处理从 TCP 连接收到的消息。逻辑与 UDP 类似，只是发送方式不同。
func (s *Server) Hand(msg []byte, conn net.Conn) {
	// 检查消息长度
	if len(msg) < 1 {
		log.Printf("Message too short from %s\n", conn.RemoteAddr().String())
		return
	}

	parentIP := s.Config.GetServerIp(conn.RemoteAddr().String())
	// 假设 msg 的第 0 字节是消息类型，第 1 字节是动作类型
	msgType := msg[0]
	msgAction := msg[1]

	switch msgType {
	case PingMsg:
		var ping Ping
		if err := tool.DecodeMsgPayload(msg, &ping); err != nil {
			log.Printf("[TCP] Failed to decode ping message: %v", err)
			return
		}

		sourceIP := tool.ByteToIPv4Port(ping.Src)
		log.Printf("[DEBUG] Received PING from %s (seq=%d)", sourceIP, ping.SeqNo)

		// 构造丰富的 ACK 响应
		ackResp := AckResp{
			SeqNo:     ping.SeqNo,
			Timestamp: time.Now().UnixNano(),
			NodeState: NodeSurvival, // 当前节点状态
			Source:    s.Config.IPBytes(),
			Payload:   []byte(fmt.Sprintf("ACK from %s", s.Config.ServerAddress)),
		}

		// 发送 ACK 响应
		s.sendAckResponse(conn, ackResp)

		// 更新发送方节点状态
		s.Member.Lock()
		meta, ok := s.Member.MetaData[sourceIP]
		if ok {
			meta.UpdateTime = time.Now().Unix()
			meta.State = NodeSurvival
		}
		s.Member.Unlock()

	case IndirectPingMsg:
		// 处理间接 PING 消息
		var ping Ping
		if err := tool.DecodeMsgPayload(msg, &ping); err != nil {
			log.Printf("[DEBUG] Failed to decode indirect ping message: %v", err)
			return
		}

		targetAddr := tool.ByteToIPv4Port(ping.Addr)
		sourceAddr := tool.ByteToIPv4Port(ping.Src)
		log.Printf("[DEBUG] Received INDIRECT_PING for %s from %s (seq=%d)",
			targetAddr, sourceAddr, ping.SeqNo)

		// 确认收到间接 PING 请求
		ackResp := AckResp{
			SeqNo:     ping.SeqNo,
			Timestamp: time.Now().UnixNano(),
			NodeState: NodeSurvival,
			Source:    s.Config.IPBytes(),
			Payload:   []byte(fmt.Sprintf("Indirect ping request received for %s", targetAddr)),
		}
		s.sendAckResponse(conn, ackResp)

		// 向目标节点发送 PING
		pingForTarget := Ping{
			SeqNo: ping.SeqNo,
			Addr:  ping.Addr,
			Src:   ping.Src, // 原始发送者
		}

		pingMsg, _ := tool.Encode(PingMsg, PingAction, &pingForTarget, false)

		// 根据连接类型选择发送方式
		if s.udpServer != nil {
			s.udpServer.UDPSendMessage(targetAddr, []byte{}, pingMsg)
		} else {
			s.SendMessage(targetAddr, []byte{}, pingMsg)
		}

		log.Printf("[DEBUG] Forwarded ping to %s on behalf of %s", targetAddr, sourceAddr)

	case AckRespMsg:
		// 处理 ACK 响应
		var ackResp AckResp
		if err := tool.DecodeMsgPayload(msg, &ackResp); err != nil {
			log.Printf("[DEBUG] Failed to decode ack response: %v", err)
			return
		}

		sourceAddr := tool.ByteToIPv4Port(ackResp.Source)
		log.Printf("[DEBUG] Received ACK (seq=%d) from %s", ackResp.SeqNo, sourceAddr)

		// 更新节点状态
		s.Member.Lock()
		meta, ok := s.Member.MetaData[sourceAddr]
		if ok {
			meta.UpdateTime = time.Now().Unix()
			meta.State = ackResp.NodeState
		}
		s.Member.Unlock()

		// 如果心跳服务存在，通知它处理 ACK
		if s.HeartbeatService != nil {
			s.HeartbeatService.handleAckResponse(ackResp)
		}

	case SuspectMsg:
		// 处理怀疑节点消息
		var suspect Suspect
		if err := tool.DecodeMsgPayload(msg, &suspect); err != nil {
			log.Printf("[DEBUG] Failed to decode suspect message: %v", err)
			return
		}

		suspectAddr := tool.ByteToIPv4Port(suspect.Addr)
		sourceAddr := tool.ByteToIPv4Port(suspect.Src)
		log.Printf("[DEBUG] Received SUSPECT message for %s from %s",
			suspectAddr, sourceAddr)

		// 如果是针对本节点的怀疑消息，发送反驳
		if suspectAddr == s.Config.ServerAddress {
			log.Println("[WARN] Received suspect message for self, sending refutation")
			ackResp := AckResp{
				SeqNo:     0, // 不需要对应序列号
				Timestamp: time.Now().UnixNano(),
				NodeState: NodeSurvival,
				Source:    s.Config.IPBytes(),
				Payload:   []byte("REFUTE_SUSPECT"),
			}

			refuteMsg, _ := tool.Encode(AckRespMsg, PingAction, &ackResp, false)
			s.SendMessage(sourceAddr, []byte{}, refuteMsg)
			return
		}

		// 更新节点状态为可疑
		s.Member.Lock()
		meta, ok := s.Member.MetaData[suspectAddr]
		if ok && meta.State == NodeSurvival {
			meta.State = NodeSuspected
			meta.UpdateTime = time.Now().Unix()

			// 广播可疑状态
			encode, _ := tool.Encode(SuspectMsg, SuspectMsg, &suspect, false)
			s.ColoringMessage(encode, SuspectMsg)
		}
		s.Member.Unlock()

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
	}
}

// sendAckResponse 发送 ACK 响应，统一处理 TCP 和 UDP
func (s *Server) sendAckResponse(conn net.Conn, ackResp AckResp) {
	// 编码 ACK 响应
	out, err := tool.Encode(AckRespMsg, PingAction, &ackResp, false)
	if err != nil {
		log.Printf("[ERROR] Failed to encode AckResp: %v", err)
		return
	}

	// 创建 4 字节 header 表示消息体长度
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(out)))

	// 合并 header 和消息体
	packet := append(header, out...)

	// 根据连接类型不同，使用不同的发送方式
	if udpConn, ok := conn.(*udpConnWrapper); ok {
		// UDP 连接
		if s.udpServer != nil {
			remoteAddr := udpConn.remoteAddr.String()
			err = s.udpServer.UDPSendMessage(remoteAddr, []byte{}, out)
		}
	} else {
		// TCP 连接
		_, err = conn.Write(packet)
	}

	if err != nil {
		log.Printf("[ERROR] Error sending ACK response: %v", err)
	} else {
		log.Printf("[DEBUG] Sent ACK response (seq=%d) to %s",
			ackResp.SeqNo, conn.RemoteAddr().String())
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

// Hand UDPServer的Hand方法实现，处理所有UDP消息
func (s *UDPServer) Hand(msg []byte, conn net.Conn) {
	// 如果有设置处理程序，则转发给它处理
	if s.H != nil && s.H != s {
		s.H.Hand(msg, conn)
	} else {
		log.Printf("[WARN] UDP: No handler set for message type: %d", msg[0])
	}
}
