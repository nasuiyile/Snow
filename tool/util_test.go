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

func Test(t *testing.T) {

	f := variance([]float64{20, 20, 20, 10, 10, 10, 0, 0, 0, 0})
	fmt.Println("方差为", f)
	f = variance([]float64{20, 20, 20, 0, 10, 0, 10, 10, 20, 10})
	fmt.Println("coloring方差为", f)
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
