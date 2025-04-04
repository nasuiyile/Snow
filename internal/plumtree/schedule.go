package plumtree

import (
	. "snow/common"
	"snow/tool"
	"time"
)

func (s *Server) lazyPushTask(stop <-chan struct{}) {
	for {
		interval := s.PConfig.LazyPushInterval
		select {
		case <-time.After(interval):
			s.lazyPush()
		case <-stop:
			return
		}
	}

}

//将所有消息缓存到队列里

func (s *Server) lazyPush() {
	//k个随机个节点，但是排除自身EagerPush里面的内容
	for bytes := range s.MessageIdQueue {
		nodes := s.KRandomNodes(s.Config.FanOut)
		for _, node := range nodes {
			s.SendIHAVE(node, bytes)
		}
	}
}
func (s *Server) SendIHAVE(ip string, msg []byte) {
	//只需要发送消息ID来确认有没有收到过消息
	s.SendMessage(ip, []byte{LazyPush, IHAVE}, msg)
}
func (s *Server) KRandomNodes(k int) []string {
	s.Member.Lock()
	defer s.Member.Unlock()
	ip := make([]string, 0)
	//当前节点的ID，需要被排除
	//把自己和eagerPush列表里的排除
	nodeIdx := make([]int, 0)
	currentIdx := s.Server.Member.Find(s.Config.IPBytes())
	nodeIdx = append(nodeIdx, currentIdx)
	s.EagerPush.Range(func(key string) bool {
		idx := s.Server.Member.Find(tool.IPv4To6Bytes(key))
		if idx != -1 {
			nodeIdx = append(nodeIdx, idx)
		}
		return true
	})

	randomNodes := tool.KRandomNodes(0, s.Member.MemberLen()-1, nodeIdx, k)
	for _, v := range randomNodes {
		ip = append(ip, tool.ByteToIPv4Port(s.Member.IPTable[v]))
	}
	return ip
}
