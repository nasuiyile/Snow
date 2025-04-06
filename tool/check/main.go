package main

import (
	"fmt"
	"snow/tool"
)

func main() {

	startPort := 40000
	endPort := 46000

	fmt.Printf("Checking ports from %d to %d...\n", startPort, endPort)
	for port := startPort; port <= endPort; port++ {
		if tool.CheckPortInUse(port) {
			fmt.Printf("Port %d is in use.\n", port)
		}
	}
	fmt.Println("completed")
}
