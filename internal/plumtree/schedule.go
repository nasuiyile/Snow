package plumtree

import (
	. "snow/common"
	"time"
)

func (s *Server) lazyPushTask(stop <-chan struct{}) {
	interval := s.PConfig.LazyPushInterval
	select {
	case <-time.After(interval):
		s.lazyPush()
	case <-stop:
		return
	}
}

//将所有消息缓存到队列里

func (s *Server) lazyPush() {
	//k个随机个节点，但是排除自身EagerPush里面的内容
	nodes := s.KRandomNodes(s.Config.FanOut)
	for bytes := range s.MessageIdQueue {
		for _, node := range nodes {
			s.SendIHAVE(node, bytes)
		}
	}
}
func (s *Server) SendIHAVE(ip string, msg []byte) {
	//只需要发送消息ID来确认有没有收到过消息
	s.SendMessage(ip, []byte{LazyPush, IHAVE}, msg)
}
