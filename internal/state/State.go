package state

import (
	"snow/tool"
	"sync"
)

type State struct {
	State *tool.TimeoutMap //记录最近收到消息的状态信息来去重

	ReliableMsgLock sync.Mutex
	//（失败）根节点接受到消息之后x秒后会检查相应的元素是否被删除了，如果没被删除则进行回调。然后删除	（成功）主动删除时如果发现自己是根节点，则也进行回调
	ReliableTimeout map[string]*ReliableInfo //可靠消息的超时
}
type ReliableInfo struct {
	IsRoot bool
	//叶子节点的计数器
	Counter int
	//需要回调的parent node
	Ip     []byte
	Action *func(isSuccess bool)
}

func (s *State) AddReliableTimeout(m []byte, isRoot bool, leafNum int, ip []byte, action *func(isSuccess bool)) {
	s.ReliableMsgLock.Lock()
	defer s.ReliableMsgLock.Unlock()
	hash := string(m)
	s.ReliableTimeout[hash] = &ReliableInfo{
		IsRoot:  isRoot,
		Counter: leafNum,
		Ip:      ip,
		Action:  action,
	}
}
func (s *State) DeleteReliableTimeout(m []byte) {
	s.ReliableMsgLock.Lock()
	defer s.ReliableMsgLock.Unlock()
	hash := string(m)
	delete(s.ReliableTimeout, hash)
}

func (s *State) GetReliableTimeout(m []byte) *ReliableInfo {
	s.ReliableMsgLock.Lock()
	defer s.ReliableMsgLock.Unlock()
	hash := string(m)
	b, ok := s.ReliableTimeout[hash]
	if !ok {
		return nil
	} else {
		return b
	}
}
