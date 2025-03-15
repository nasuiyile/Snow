package main

import (
	"encoding/json"
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
	arr := make([]int, 0)
	for k := 2; k < 40; k = k + 2 {
		for i := 1; i < 1000; i++ {
			if k > i {
				continue
			}

			tree := createTree(i, k)
			values := getNonLeafNodes(tree)
			//没有leaf的节点，我们希望所有节点+1一定是叶子节点=
			for _, v := range values {
				v++
				if v >= i {
					v = 0
				}
				node := findNode(tree, v)
				if node == nil {
					return
				}
				if node.Left != nil || node.Right != nil {
					fmt.Println("出问题的值！i:", i, "k:", k, "v", v)
					if i%2 == 1 {
						arr = append(arr, i)
					}
				}
			}
		}

	}
	tree := createTree(90, 2)
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
		//只有一个节点，返回当前
		node.Current = left
		return &node
	} else if n <= k {
		//左右都要有，右边没有元素的情况下放唯一一个放左边
		for i := 0; i < n; i++ {
			if i%2 == 0 {
				node.Left = append(node.Left, &Node{Current: left})
				left++
			} else {
				node.Right = append(node.Right, &Node{Current: right})
				right--
			}

		}
		if len(node.Left) == 1 {
			node.Current = node.Right[0].Current
			node.Right = nil
		} else {
			node.Current = node.Left[len(node.Left)-1].Current
			node.Left = node.Left[:len(node.Left)-1]
			if len(node.Left) == 0 {
				node.Left = nil
			}
		}

		return &node
	}
	leftRange := make([]area, 0)
	rightRange := make([]area, 0)
	//h是总高度-1，也就是说h一定是满二叉树
	h := getFullHeight(k, n)
	//当前节点所在的子树有多少个节点（子树不包括根节点）
	//这样我们就能和最后一层一起计算当前节点负责的区域了
	subFull := getFullTreeNum(k, h-1)
	//当前已经被填充满的大小（包括根节点）
	fullTree := getFullTreeNum(k, h)
	//子树的最后一层的
	nextLevel := (getFullTreeNum(k, h+1) - fullTree) / k
	//最后一层多少节点
	totalBottom := n - fullTree
	//factor代表能覆盖多完少子树的最后一层
	factor := totalBottom / nextLevel
	//覆盖完所有子树还留下来多少
	remain := totalBottom % nextLevel
	leftBound := 0
	for i := 1; i < k/2+1; i++ {
		start := leftBound
		if factor >= 1 {
			//如果能覆盖的化直接用nextLevel覆盖掉，因为这一层是满的
			leftBound = leftBound + nextLevel + subFull - 1
		} else if factor == 0 {
			//因为factor一直在减少，所以这里是第一次因子为0的时候，把remain补上去
			leftBound = leftBound + remain + subFull - 1
		} else {
			//之后的几层就是没有remain的
			leftBound = leftBound + subFull - 1

		}
		//下一次递归的长度
		nextN := leftBound - start + 1
		//这个行代码是为了把多余的一个节点放在最最右边节点的左子树上
		if nextN%2 == 0 && nextN >= 2 {
			leftBound--
		}
		leftRange = append(leftRange, area{Left: start, Right: leftBound})
		node.Left = append(node.Left, createSubTree(start+left, leftBound+left, k))
		leftBound++
		factor--
	}
	node.Current = leftBound + left
	//下面是一样的，对右边区域开始操作
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
		if i >= k/2 {
			node.Right = append(node.Right, createSubTree(start+left, right, k))
		} else {
			node.Right = append(node.Right, createSubTree(start+left, leftBound+left, k))
		}
		leftBound++
		factor--
	}

	return &node
}

// 获取对应高度下的总数，后续可以优化
func getFullTreeNum(k int, h int) int {
	if h == 1 {
		return 1
	}
	h--
	return int((math.Pow(float64(k), float64(h+1)))-1) / (k - 1)
}

// 获取对应总数下的高度
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

}
