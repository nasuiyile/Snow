package tool

import (
	"fmt"
	"net"
	"os/exec"
	"snow/common"
	"strconv"
	"strings"
)

func CheckPortInUse(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return true // 端口被占用
	}
	ln.Close()
	return false // 端口未被占用
}

func DisablePort(port int) error {
	portStr := strconv.Itoa(port)
	cmdStr := "iptables -A INPUT -p tcp --sport {port} -j DROP && " +
		"iptables -A OUTPUT -p tcp --sport {port} -j DROP && " +
		"sudo iptables -A INPUT -p tcp --dport {port} -j DROP && " +
		"sudo iptables -A OUTPUT -p tcp --dport {port} -j DROP"
	cmd := strings.Replace(cmdStr, "{port}", portStr, -1)
	// 执行命令
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		fmt.Println("Error executing command:", err)
	}
	fmt.Println(string(out))
	return err
}
func DisableNode(port int) {
	// 关闭服务器和客户端的端口
	if CheckPortInUse(port) {
		err := DisablePort(port)
		if err != nil {
			panic(err)
		}
	}
	if CheckPortInUse(port + common.Offset) {
		err := DisablePort(port + common.Offset)
		if err != nil {
			panic(err)
		}
	}
}
