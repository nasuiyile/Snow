package main

import (
	"fmt"
	"snow/util"
)

func main() {

	startPort := 20000
	endPort := 46000

	fmt.Printf("Checking ports from %d to %d...\n", startPort, endPort)
	for port := startPort; port <= endPort; port++ {
		if util.CheckPortInUse(port) {
			fmt.Printf("Port %d is in use.\n", port)
		}
	}
	fmt.Println("completed")
}
