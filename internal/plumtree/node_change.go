package plumtree

import (
	. "snow/common"
)

func (s *Server) NodeJoin() {
	s.PlumTreeBroadcast([]byte("nodejoinwehellosdfds"), NodeJoin)
}

func (s *Server) NodeLeave() {
	s.PlumTreeBroadcast([]byte{}, NodeLeave)
}
