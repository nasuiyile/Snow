package plumtree

import (
	. "snow/common"
	"snow/util"
)

//将所有消息缓存到队列里

func (s *Server) lazyPush(bytes []byte) {
	//k个随机个节点，但是排除自身EagerPush里面的内容

	nodes := s.KRandomNodes(s.Config.FanOut)
	for _, node := range nodes {
		s.SendIHAVE(node, bytes)
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
	currentIdx := s.Server.Member.Find(s.Config.IPBytes(), false)
	nodeIdx = append(nodeIdx, currentIdx)
	s.EagerPush.Range(func(key string) bool {
		idx := s.Server.Member.Find(util.IPv4To6Bytes(key), false)
		if idx != -1 {
			nodeIdx = append(nodeIdx, idx)
		}
		return true
	})

	randomNodes := util.KRandomNodes(0, s.Member.MemberLen()-1, nodeIdx, k)
	for _, v := range randomNodes {
		ip = append(ip, util.ByteToIPv4Port(s.Member.IPTable[v]))
	}
	return ip
}
