package tool

import (
	"fmt"
	"os/exec"
)

func exe() {
	// 需要执行的命令
	cmd := exec.Command("cmd", "/C", "echo Hello, World!")

	// 获取命令的输出
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// 打印输出
	fmt.Println(string(output))
}
