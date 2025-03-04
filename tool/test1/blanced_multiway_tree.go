package main

import (
	"encoding/json"
	"fmt"
)

func findNode(root *Node, target int) *Node {
	if root == nil {
		return nil
	}

	// 如果当前节点的值等于目标值，返回该节点
	if root.Current == target {
		return root
	}

	// 优先在 Left 列表中查找
	for _, leftNode := range root.Left {
		found := findNode(leftNode, target)
		if found != nil {
			return found
		}
	}

	// 然后在 Right 列表中查找
	for _, rightNode := range root.Right {
		found := findNode(rightNode, target)
		if found != nil {
			return found
		}
	}

	// 如果没有找到，返回 nil
	return nil
}

func getNonLeafNodes(root *Node) []int {
	var result []int
	if root == nil {
		return result
	}
	if len(root.Left) > 0 || len(root.Right) > 0 {
		result = append(result, root.Current)
	}
	for _, leftChild := range root.Left {
		result = append(result, getNonLeafNodes(leftChild)...) // 递归左子树
	}
	for _, rightChild := range root.Right {
		result = append(result, getNonLeafNodes(rightChild)...) // 递归右子树
	}
	return result
}
func getNonLeafNodes2(area int, root int) []int {
	//奇偶性要和根节点相反
	i := root%2 + 1
	var result []int
	for ; i < area; i = i + 2 {
		result = append(result, i)
	}
	return result
}
func main() {
	//arr := make([]int, 0)
	//for k := 2; k < 40; k = k + 2 {
	//	for i := 1; i < 100; i = i + 1 {
	//		if k > i {
	//			continue
	//		}
	//		if k == 2 && i == 8 {
	//			fmt.Println()
	//		}
	//		tree := createTree(i, k)
	//		values := getNonLeafNodes2(i, tree.Current)
	//		//没有leaf的节点，我们希望所有节点+1一定是叶子节点=
	//		for _, v := range values {
	//			node := findNode(tree, v)
	//			if node == nil {
	//				return
	//			}
	//			if node.Left != nil || node.Right != nil {
	//				fmt.Println("出问题的值！i:", i, "k:", k, "v", v, "current:", node.Current)
	//				if i%2 == 1 {
	//					arr = append(arr, i)
	//				}
	//			}
	//		}
	//	}
	//
	//}
	tree := createTree(10, 2)
	fmt.Println(tree)
	marshal, _ := json.Marshal(tree)
	fmt.Println(string(marshal))
	//fmt.Println(arr)
	//values := getNonLeafNodes(tree)

}

type Node struct {
	Current int `json:"current"`
	Left    []*Node
	Right   []*Node
}

func createTree(n int, k int) *Node {
	return createSubTree(0, n-1, (n)/2, k)
}

// 除了根节点外，每次都会指定current
func createSubTree(left int, right int, current int, k int) *Node {
	node := Node{Current: current}
	if left > right {
		return nil
	} else if left == right {
		//只有一个节点，返回当前
		return &node
	} else if (right - left + 1) < k {
		for ; left < current; left++ {
			node.Left = append(node.Left, &Node{Current: left})
		}
		for ; current < right; current++ {
			current++
			node.Right = append(node.Right, &Node{Current: current})
		}
		return &node
	}

	leftArea := (current - left) / (k / 2)
	leftRemain := (current - left) % (k / 2)
	previousScope := left
	for i := 0; i < k/2; i++ {
		//将多余的区域从左边开始均分给每一个节点
		currentArea := leftArea
		if leftRemain > 0 {
			leftRemain--
			currentArea++
		}
		rightBound := previousScope + currentArea - 1
		leftNodeValue := (previousScope + (rightBound + 1)) / 2
		if leftNodeValue == 2 {
			fmt.Println()
		}

		node.Left = append(node.Left, createSubTree(previousScope, rightBound, leftNodeValue, k))
		previousScope = rightBound + 1

	}
	previousScope = current + 1
	rightArea := (right - current) / (k / 2)
	rightRemain := (right - current) % (k / 2)
	for i := 0; i < k/2; i++ {
		//将多余的区域从左边开始均分给每一个节点
		currentArea := rightArea
		if rightRemain > 0 {
			rightRemain--
			currentArea++
		}
		rightBound := previousScope + currentArea - 1
		rightNodeValue := (previousScope + (rightBound + 1)) / 2

		node.Right = append(node.Right, createSubTree(previousScope, rightBound, rightNodeValue, k))
		previousScope = rightBound + 1
	}

	return &node
}
