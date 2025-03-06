package main

import (
	"fmt"
	"snow/internal/broadcast"
)

func main() {
	tree := broadcast.CreateSubTree(0, 9, 5, 10, 2)
	fmt.Println(tree)
}
