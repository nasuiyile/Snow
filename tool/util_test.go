package tool

import (
	"fmt"
	"testing"
)

func TestGetRandomExcluding(t *testing.T) {
	excluding := GetRandomExcluding(0, 10, 4, 10)
	fmt.Println(excluding)
}
