package broadcast

import (
	"bytes"
	"fmt"
	"net"
	. "snow/common"
	"snow/tool"
	"time"

	log "github.com/sirupsen/logrus"
)

// Hand 处理从 TCP 连接收到的消息。逻辑与 UDP 类似，只是发送方式不同。
func (s *Server) Hand(msg []byte, conn net.Conn) {
	// 检查消息长度
	if len(msg) < 1 {
		log.Warn("Message too short from %s\n", conn.RemoteAddr().String())
		return
	}
	if s.IsClosed.Load() {
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
			log.Infof("Failed to decode ping message: %v", err)
			return
		}

		sourceIP := tool.ByteToIPv4Port(ping.Src)
		localAddr := conn.LocalAddr().String()
		log.Infof("[INFO] Node %s 收到 PING 请求 (seq=%d) 来自 %s",
			localAddr,  // 接收方地址
			ping.SeqNo, // 序列号
			sourceIP)   // 发送方地址

		// 构造丰富的 ACK 响应
		ackResp := AckResp{
			SeqNo:      ping.SeqNo,
			Timestamp:  time.Now().UnixNano(),
			NodeState:  NodeSurvival, // 当前节点状态
			Source:     s.Config.IPBytes(),
			IsIndirect: false,
		}

		log.Infof("[INFO] Node %s 准备回复 ACK (seq=%d) 给 %s",
			localAddr, ping.SeqNo, sourceIP)

		// 发送 ACK 响应
		s.sendAckResponse(conn, ackResp, tool.ByteToIPv4Port(ping.Src))

		// 更新发送方节点状态
		s.Member.Lock()
		meta, ok := s.Member.MetaData[sourceIP]
		if ok {
			meta.UpdateTime = time.Now().Unix()
			meta.State = NodeSurvival
		}
		s.Member.Unlock()

	case IndirectPingMsg:
		var ping Ping
		if err := tool.DecodeMsgPayload(msg, &ping); err != nil {
			log.Debugf("[DEBUG] Failed to decode indirect ping message: %v", err)
			return
		}

		targetAddr := tool.ByteToIPv4Port(ping.Addr)
		sourceAddr := tool.ByteToIPv4Port(ping.Src)

		// 确认收到间接 PING 请求，设置 IsIndirect 为 true
		ackResp := AckResp{
			SeqNo:      ping.SeqNo,
			Timestamp:  time.Now().UnixNano(),
			NodeState:  NodeSurvival,
			Source:     s.Config.IPBytes(),
			IsIndirect: true,
		}
		s.sendAckResponse(conn, ackResp, sourceAddr)

		// 转发ping到目标节点，使用当前节点作为源地址
		forwardPing := Ping{
			SeqNo: ping.SeqNo,
			Addr:  ping.Addr,
			Src:   s.Config.IPBytes(), // 使用当前节点的地址作为源
		}

		forwardPingData, err := tool.Encode(PingMsg, PingAction, &forwardPing, false)
		if err != nil {
			log.Errorf("[ERR] Failed to encode forwardPing: %v", err)
			return
		}

		// 检查 UDP Server 是否可用
		if s.UdpServer == nil {
			log.Warnf("[WARN] UDP Server not available, cannot forward ping to %s", targetAddr)
			return
		}

		if err := s.UdpServer.UDPSendMessage(targetAddr, []byte{}, forwardPingData); err != nil {
			log.Errorf("[ERR] Failed to forward ping to %s: %v", targetAddr, err)
			return
		}

		log.Printf("[INFO] Forward ping to %s", targetAddr)

		// 注册回调处理目标节点的响应
		if s.HeartbeatService != nil {
			s.HeartbeatService.registerAckHandler(ping.SeqNo, func(targetAckResp AckResp) {
				// 创建转发的 ACK，设置 IsIndirect 为 false
				forwardAck := AckResp{
					SeqNo:      ping.SeqNo,
					Timestamp:  targetAckResp.Timestamp,
					NodeState:  targetAckResp.NodeState,
					Source:     targetAckResp.Source,
					IsIndirect: false,
				}

				forwardAckData, err := tool.Encode(AckRespMsg, PingAction, &forwardAck, false)
				if err != nil {
					log.Errorf("[ERR] Failed to encode forwardAck: %v", err)
					return
				}

				if err := s.UdpServer.UDPSendMessage(sourceAddr, []byte{}, forwardAckData); err != nil {
					log.Errorf("[ERR] Failed to forward ACK to %s: %v", sourceAddr, err)
					return
				}

				log.Printf("[DEBUG] Successfully forwarded ACK from %s to %s (seq=%d)",
					targetAddr, sourceAddr, ping.SeqNo)
			}, 8*time.Second, nil)
		} else {
			log.Printf("[DEBUG] HeartbeatService not available, skipping ack handler registration")
		}

		log.Printf("[DEBUG] Successfully forwarded ping to %s on behalf of %s", targetAddr, sourceAddr)

	case AckRespMsg:
		// 处理 ACK 响应
		var ackResp AckResp
		if err := tool.DecodeMsgPayload(msg, &ackResp); err != nil {
			log.Debugf("[DEBUG] Failed to decode ack response: %v", err)
			return
		}

		sourceAddr := tool.ByteToIPv4Port(ackResp.Source)
		log.Infof("[INFO] Node %s 收到 ACK 响应 (seq=%d) 来自 %s, isIndirect=%v",
			s.Config.ServerAddress, ackResp.SeqNo, sourceAddr, ackResp.IsIndirect)

		// 更新节点状态
		s.Member.Lock()
		meta, ok := s.Member.MetaData[sourceAddr]
		if ok {
			meta.UpdateTime = time.Now().Unix()
			meta.State = ackResp.NodeState
			log.Infof("[INFO] 已更新节点 %s 的状态和时间戳, 序列号=%d", sourceAddr, ackResp.SeqNo)
		} else {
			log.Warnf("[WARN] 找不到节点 %s 的元数据, ACK seq=%d", sourceAddr, ackResp.SeqNo)
		}
		s.Member.Unlock()

		// 如果心跳服务存在，通知它处理 ACK
		if s.HeartbeatService != nil {
			log.Infof("[INFO] 转发 ACK (seq=%d) 到心跳服务进行处理", ackResp.SeqNo)
			s.HeartbeatService.handleAckResponse(ackResp)
		} else {
			log.Warnf("[WARN] 心跳服务不可用，无法处理 ACK (seq=%d)", ackResp.SeqNo)
		}

	case RegularMsg:
		body := tool.CutBytes(msg)
		if !IsFirst(body, msgType, msgAction, s) {
			return
		}
		forward(msg, s, parentIP)
	case ColoringMsg:
		body := tool.CutBytes(msg)
		first := IsFirst(body, msgType, msgAction, s)
		if first {
			if msgAction == ReportLeave {
				leaveNode := tool.CutTimestamp(body)
				if tool.ByteToIPv4Port(leaveNode) != "127.0.0.1:40026" && s.Config.ServerAddress != "127.0.0.1:40026" {
					fmt.Println(conn.RemoteAddr().String())
					fmt.Println(tool.ByteToIPv4Port(leaveNode))
				}
				s.Member.RemoveMember(leaveNode, false)
			}
		}
		forward(msg, s, parentIP)
		if !first {
			return
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
func (s *Server) sendAckResponse(conn net.Conn, ackResp AckResp, targetAddr string) {
	// 编码 ACK 响应
	out, err := tool.Encode(AckRespMsg, PingAction, &ackResp, false)
	if err != nil {
		log.Errorf("[ERROR] Failed to encode AckResp: %v", err)
		return
	}

	// 根据连接类型不同，使用不同的发送方式
	if _, ok := conn.(*udpConnWrapper); ok {
		// UDP 连接
		if s.UdpServer != nil {
			err = s.UdpServer.UDPSendMessage(targetAddr, []byte{}, out)
			log.Infof("[INFO] Node %s 发送 ACK (seq=%d) 给 %s (通过UDP)",
				conn.LocalAddr().String(), ackResp.SeqNo, targetAddr)
		}
	} else {
		// TCP 连接
		s.SendMessage(targetAddr, []byte{}, out)
		log.Infof("[INFO] Node %s 发送 ACK (seq=%d) 给 %s (通过TCP)",
			s.Config.ServerAddress, ackResp.SeqNo, targetAddr)
	}

	if err != nil {
		log.Errorf("[ERROR] 发送 ACK (seq=%d) 给 %s 失败: %v",
			ackResp.SeqNo, targetAddr, err)
	} else {
		log.Infof("[INFO] 成功发送 ACK (seq=%d) 给 %s",
			ackResp.SeqNo, targetAddr)
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
			//newMsg = append(newMsg, msg[len(msg)-IpLen:]...)
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
		log.Warn("[WARN] UDP: No handler set for message type: %d", msg[0])
	}
}
