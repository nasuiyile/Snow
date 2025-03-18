package tool

import (
	"fmt"
	"math"
	"testing"
)

func TestGetRandomExcluding(t *testing.T) {
	excluding := KRandomNodes(0, 10, 4, 10)
	fmt.Println(excluding)
}

// Action 接收到消息后执行的操作，不允许对原byte进行修改。不需要的操作可以不进行设置
type Action struct {
}

func (a *Action) init() {
	//逻辑
	a.func1()
}
func (a *Action) func1() {
	//逻辑

}

type ActionSon struct {
	Action
}

func (a *ActionSon) init() {
	//逻辑
	a.Action.init()
}

func (a *ActionSon) func1() {
	//逻辑

}

func Test(t *testing.T) {

}

// variance 计算一组数的方差
func variance(nums []float64) float64 {
	n := float64(len(nums))
	if n == 0 {
		return 0
	}

	// 计算均值
	sum := 0.0
	for _, num := range nums {
		sum += num
	}
	mean := sum / n

	// 计算方差
	var sumSquaredDiffs float64
	for _, num := range nums {
		sumSquaredDiffs += math.Pow(num-mean, 2)
	}

	return sumSquaredDiffs / n
}
