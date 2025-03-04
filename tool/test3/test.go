package main

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// 获取所有叶子节点的 current 值
func getLeafValues(node Node) []int {
	var result []int

	// 定义递归函数
	var dfs func(n Node)
	dfs = func(n Node) {
		if len(n.Left) == 0 && len(n.Right) == 0 { // 判断是否为叶子节点
			result = append(result, n.Current)
			return
		}

		// 递归遍历 Left 子树
		for _, child := range n.Left {
			dfs(child)
		}

		// 递归遍历 Right 子树
		for _, child := range n.Right {
			dfs(child)
		}
	}

	// 开始递归
	dfs(node)
	return result
}
func PrintTree(root Node) {
	if reflect.DeepEqual(root, Node{}) {
		fmt.Println("Empty tree")
		return
	}

	// 使用队列进行层级遍历
	queue := []Node{root}

	for len(queue) > 0 {
		levelSize := len(queue)
		levelNodes := []int{}

		for i := 0; i < levelSize; i++ {
			// 取出队首元素
			node := queue[0]
			queue = queue[1:]

			// 记录当前节点的值
			levelNodes = append(levelNodes, node.Current)

			// 将左右子节点加入队列
			queue = append(queue, node.Left...)
			queue = append(queue, node.Right...)
		}

		// 打印当前层的节点值
		fmt.Println(levelNodes)
	}

}
func findNode(root Node, target int) *Node {
	// 检查当前节点是否为目标节点
	if root.Current == target {
		return &root
	}

	// 遍历左子树
	for _, leftNode := range root.Left {
		result := findNode(leftNode, target)
		if result != nil {
			return result
		}
	}

	// 遍历右子树
	for _, rightNode := range root.Right {
		result := findNode(rightNode, target)
		if result != nil {
			return result
		}
	}

	// 如果没有找到目标节点，返回 nil
	return nil
}

func main() {
	//for i := 1; i < 300; i++ {
	//	for k := 4; k < 20; k = k + 2 {
	//		tree := createTree(i, k)
	//		//PrintTree(tree)
	//		if i == 9 {
	//			print()
	//		}
	//		values := getLeafValues(tree)
	//		for _, v := range values {
	//			v--
	//			if v < 0 {
	//				v = i - 1
	//			}
	//
	//			node := findNode(tree, v)
	//			if node.Left != nil || node.Right != nil {
	//				fmt.Println("出问题了兄弟！", k, i)
	//			}
	//		}
	//	}
	//
	//}
	tree := createTree(35, 4)
	PrintTree(tree)
	marshal, _ := json.Marshal(tree)
	fmt.Println(string(marshal))
}

type Node struct {
	Current int `json:"current"`
	Left    []Node
	Right   []Node
}

func createTree(p int, k int) Node {
	left := p / 2
	current := left
	node := Node{Current: current, Left: make([]Node, 0), Right: make([]Node, 0)}
	leftNodes := createHalfTree(0, current-1, k)
	node.Left = append(node.Left, leftNodes...)
	rightNodes := createHalfTree(current+1, p, k)
	node.Right = append(node.Right, rightNodes...)
	return node
}
func createHalfTree(start int, end int, k int) []Node {
	if end-start < 0 {
		return nil
	}
	nodes := make([]Node, 0)
	distance := (end - start) / (k / 2)
	if distance == 0 {
		for end >= start {
			nodes = append(nodes, Node{Current: start})
			start++
		}
		return nodes
	}
	for i := 1; i <= k/2; i++ {
		if i != k/2 {
			left := start + (i-1)*distance
			right := start + i*distance - 1
			mid := (left + right) / 2
			node := Node{Current: mid, Left: createHalfTree(left, mid-1, k), Right: createHalfTree(mid+1, right, k)}
			nodes = append(nodes, node)
		} else {
			left := start + (i-1)*distance
			right := end
			mid := (left + right) / 2
			node := Node{Current: mid, Left: createHalfTree(left, mid-1, k), Right: createHalfTree(mid+1, right, k)}
			nodes = append(nodes, node)
		}
	}

	return nodes
}
