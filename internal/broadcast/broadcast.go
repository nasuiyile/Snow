package broadcast

import (
	"math/rand"
	. "snow/common"
	"snow/tool"
	"time"
)

func (s *Server) RegularMessage(message []byte, msgAction MsgAction) {
	member, _ := s.InitMessage(RegularMsg, msgAction)
	newMsg := tool.CopyMsg(message)
	for ip, payload := range member {
		s.SendMessage(ip, payload, newMsg)
	}
}

func (s *Server) ColoringMessage(message []byte, msgAction MsgAction) {
	member, _ := s.InitMessage(ColoringMsg, msgAction)
	newMsg := tool.CopyMsg(message)
	for ip, payload := range member {
		s.SendMessage(ip, payload, newMsg)
	}
}

// GossipMessage gossip协议是有可能广播给发送给自己的节点的= =
func (s *Server) GossipMessage(msg []byte, msgAction MsgAction) {
	bytes := make([]byte, len(msg)+TimeLen+TagLen)
	bytes[0] = GossipMsg
	bytes[1] = msgAction
	copy(bytes[TagLen:], tool.RandomNumber())
	copy(bytes[TagLen+TimeLen:], msg)
	s.SendGossip(bytes)
}

func (s *Server) SendGossip(msg []byte) {
	nodes := s.KRandomNodes(s.Config.FanOut, nil)
	for _, v := range nodes {
		s.SendMessage(v, []byte{}, msg)
	}
}

// ForwardMessage 转发消息
func (s *Server) ForwardMessage(msg []byte, member map[string][]byte) {
	newMsg := tool.CopyMsg(msg)
	for ip, payload := range member {
		s.SendMessage(ip, payload, newMsg[len(payload):])
	}
}

// ReliableMessage 在成功时，每个节点都会回调这个方法，在失败时只有根节点和部分成功的节点会重新调用这个方法
func (s *Server) ReliableMessage(message []byte, msgAction MsgAction, action *func(isSuccess bool)) {

	member, randomNum := s.InitMessage(ReliableMsg, msgAction)

	data := append(randomNum, message...)
	hash := []byte(tool.Hash(data))
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
	msg := tool.CopyMsg(message)
	for ip, payload := range member {
		s.SendMessage(ip, payload, msg)
	}
}

func (s *Server) KRandomNodes(k int, exclude []byte) []string {
	s.Member.Lock()
	defer s.Member.Unlock()
	ip := make([]string, 0)
	//当前节点的ID，需要被排除
	idx := s.Member.Find(s.Config.IPBytes(), false)
	var randomNodes []int
	if exclude != nil && len(exclude) != 0 {
		excludeIdx := s.Member.Find(exclude, false)
		randomNodes = tool.KRandomNodes(0, s.Member.MemberLen()-1, []int{idx, excludeIdx}, k)
	} else {
		randomNodes = tool.KRandomNodes(0, s.Member.MemberLen()-1, []int{idx}, k)
	}

	for _, v := range randomNodes {
		ip = append(ip, tool.ByteToIPv4Port(s.Member.IPTable[v]))
	}
	return ip
}

func (s *Server) pushTrigger(stop <-chan struct{}) {
	interval := s.Config.PushPullInterval
	// Use a random stagger to avoid syncronizing
	randStagger := time.Duration(uint64(rand.Int63()) % uint64(interval))
	select {
	case <-time.After(randStagger):
	case <-stop:
		return
	}
	// Tick using a dynamic timer
	for {

		tickTime := tool.PushScale(interval, s.Member.MemberLen())
		select {
		case <-time.After(tickTime):
			s.PushState()
		case <-stop:
			return
		}
	}
}

// pushPull is invoked periodically to randomly perform a complete state
// exchange. Used to ensure a high level of convergence, but is also
// reasonably expensive as the entire state of this node is exchanged
// with the other node.
func (s *Server) PushState() {
	// Get a random live node
	nodes := s.KRandomNodes(1, nil)
	// If no nodes, bail
	if len(nodes) == 0 {
		return
	}
	node := nodes[0]
	// Attempt a push pull
	state := s.exportState()
	msg := tool.PackTagToHead(NodeChange, RegularStateSync, state)
	s.SendMessage(node, []byte{}, msg)
}
