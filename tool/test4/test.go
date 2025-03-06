package main

import (
	"fmt"
	"snow/internal/broadcast"
)

func main() {
	tree, _ := broadcast.CreateSubTree(5, 9, 9, 10, 2, true)
	fmt.Println(tree)
}
