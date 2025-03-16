package main

import (
	"fmt"
	"net"
)

func checkPortInUse(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return true // 端口被占用
	}
	ln.Close()
	return false // 端口未被占用
}

func main() {

	startPort := 40000
	endPort := 46000

	fmt.Printf("Checking ports from %d to %d...\n", startPort, endPort)
	for port := startPort; port <= endPort; port++ {
		if checkPortInUse(port) {
			fmt.Printf("Port %d is in use.\n", port)
		}
	}
}
