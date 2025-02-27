package main

import (
	"fmt"
	"math"
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
func main() {
	//
	//for k := 2; k < 20; k = k + 2 {
	//	for i := 1; i < 50; i++ {
	//		if k > i {
	//			continue
	//		}
	//		if k == 2 && i == 8 {
	//			fmt.Println()
	//		}
	//		tree := createTree(i, k)
	//		values := getNonLeafNodes(tree)
	//		for _, v := range values {
	//			v++
	//			if v >= i {
	//				v = 0
	//			}
	//			node := findNode(tree, v)
	//			if node == nil {
	//				return
	//			}
	//			if node.Left != nil || node.Right != nil {
	//				fmt.Println("出问题了兄弟！k:", k, "i:", i)
	//				fmt.Println("出问题了的值！", node.Current)
	//			}
	//		}
	//	}
	//
	//}
	tree := createTree(11, 4)
	fmt.Println(tree)
	//values := getNonLeafNodes(tree)
	//for _, v := range values {
	//	v++
	//	if v >= 6 {
	//		v = 0
	//	}
	//	node := findNode(tree, v)
	//	if node == nil {
	//		return
	//	}
	//	if node.Left != nil || node.Right != nil {
	//		fmt.Println("出问题了的值！", node.Current)
	//	}
	//}

}

type Node struct {
	Current int `json:"current"`
	Left    []*Node
	Right   []*Node
}
type area struct {
	Left  int
	Right int
}

func createTree(n int, k int) *Node {
	return createSubTree(0, n-1, k)
}

func createSubTree(left int, right int, k int) *Node {
	n := right - left + 1
	node := Node{}
	if left > right {
		return nil
	} else if left == right {
		node.Current = left
		return &node
	} else if n < k {
		if right > (left + k/2) {
			right = left + k/2
		} else {
			node.Current = right
		}
		for ; left < right; left++ {
			if len(node.Left) < k/2 {
				node.Left = append(node.Left, &Node{Current: left})
			} else {
				node.Right = append(node.Right, &Node{Current: left})
			}
		}
		return &node
	}
	leftRange := make([]area, 0)
	rightRange := make([]area, 0)
	//h是满二叉树的高度，不是总高度
	h := getFullHeight(k, n)
	//因为是满k叉树，所以下一层的数量为	（subFull+1）* k

	subFull := getFullTreeNum(k, h-1)
	//当前已经被填充满的大小
	fullTree := getFullTreeNum(k, h)
	//子树的最后一层的
	nextLevel := (getFullTreeNum(k, h+1) - fullTree) / k

	totalBottom := n - fullTree
	factor := totalBottom / nextLevel
	remain := totalBottom % nextLevel
	leftBound := 0
	for i := 1; i < k/2+1; i++ {
		start := leftBound
		if factor >= 1 {
			leftBound = leftBound + nextLevel + subFull - 1
		} else if factor == 0 {
			leftBound = leftBound + remain + subFull - 1
		} else {
			leftBound = leftBound + subFull - 1
		}
		leftRange = append(leftRange, area{Left: start, Right: leftBound})
		node.Left = append(node.Left, createSubTree(start+left, leftBound+left, k))
		leftBound++
		factor--
	}

	node.Current = leftBound + left
	if node.Current == 2 {
		fmt.Println()
	}
	leftBound++
	for i := 1; i < k/2+1; i++ {
		start := leftBound
		if factor >= 1 {
			leftBound = leftBound + nextLevel + subFull - 1
		} else if factor == 0 {
			leftBound = leftBound + remain + subFull - 1
		} else {
			leftBound = leftBound + subFull - 1
		}
		rightRange = append(rightRange, area{Left: start, Right: leftBound})
		node.Right = append(node.Right, createSubTree(start+left, leftBound+left, k))
		leftBound++
		factor--
	}

	return &node
}

func getFullTreeNum(k int, h int) int {
	if h == 1 {
		return 1
	}
	h--
	return int((math.Pow(float64(k), float64(h+1)))-1) / (k - 1)
}
func getFullHeight(k int, n int) int {
	for i := 0; i < 10000000; i++ {
		full := getFullTreeNum(k, i)
		if full == n {
			return i
		} else if full > n {
			return i - 1
		}
	}
	return -1
	//return int(math.Log(float64(n)) / math.Log(float64(k)))
}

func min(a int, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func max(a int, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}
