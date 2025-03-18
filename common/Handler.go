package common

import "net"

type HandlerFunc interface {
	Hand(msg []byte, conn net.Conn)
}
