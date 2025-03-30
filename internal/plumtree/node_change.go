package plumtree

import (
	. "snow/common"
)

func (s *Server) nodeJoin() {
	s.PlumTreeBroadcast([]byte{}, NodeJoin)
}

func (s *Server) nodeLeave() {
	s.PlumTreeBroadcast([]byte{}, NodeLeave)
}
