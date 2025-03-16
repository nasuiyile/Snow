//go:build darwin

package dialer

import (
	"fmt"
	"net"
	"syscall"
	"time"
)

func Dialer(clientAddress *net.TCPAddr, TCPTimeout time.Duration) net.Dialer {
	return net.Dialer{
		LocalAddr: clientAddress,
		Timeout:   TCPTimeout,
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				if err != nil {
					fmt.Println("设置 SO_REUSEADDR 失败:", err)
				}
			})
		},
	}
}
