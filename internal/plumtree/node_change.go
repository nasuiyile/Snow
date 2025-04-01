package plumtree

import (
	. "snow/common"
)

func (s *Server) ApplyJoin() {
	s.PlumTreeBroadcast([]byte{}, NodeJoin)
}

func (s *Server) ApplyLeave() {
	s.PlumTreeBroadcast([]byte{}, NodeLeave)
}
