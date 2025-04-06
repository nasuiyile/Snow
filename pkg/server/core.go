package server

import (
	. "snow/common"
	. "snow/config"
	"snow/internal/broadcast"
)

type SnowServer struct {
	server *broadcast.Server
}

func (s *SnowServer) ApplyJoin(ip string) {
	s.ApplyJoin(ip)
}
func (s *SnowServer) ApplyLeave() {
	s.server.ApplyLeave()
}
func (s *SnowServer) ColoringMessage(message []byte) {
	s.server.ColoringMessage(message, UserMsg)
}
func (s *SnowServer) RegularMessage(message []byte) {
	s.server.RegularMessage(message, UserMsg)
}
func (s *SnowServer) ReliableMessage(message []byte, action *func(isSuccess bool)) {
	s.server.ReliableMessage(message, UserMsg, action)
}

func (s *SnowServer) UnicastMessage(message []byte, ip string) {
	s.server.SendMessage(ip, []byte{UnicastMsg, UserMsg}, message)
}

func NewSnowServer(config *Config, action broadcast.Action) (*SnowServer, error) {
	oldServer, err := broadcast.NewServer(config, action)
	if err != nil {
		return nil, err
	}
	//扩展原来的handler方法
	server := &SnowServer{
		server: oldServer,
	}

	return server, nil
}
