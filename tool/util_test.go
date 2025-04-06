package tool

import (
	"fmt"
	"testing"
)

func TestGetRandomExcluding(t *testing.T) {
	excluding := KRandomNodes(0, 10, []int{4, 2}, 3)
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
	DisableNode(8111)

}
