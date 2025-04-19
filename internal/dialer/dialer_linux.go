//go:build linux

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
				// 设置 SO_REUSEADDR 和 SO_REUSEPORT
				err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				if err != nil {
					fmt.Println("设置 SO_REUSEADDR 失败:", err)
					return
				}
				// 设置 SO_REUSEPORT，使用 15 作为常量值
				err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, 15, 1) // 15 是 SO_REUSEPORT 的常量值
				if err != nil {
					fmt.Println("设置 SO_REUSEPORT 失败:", err)
				}
			})
		},
	}
}
