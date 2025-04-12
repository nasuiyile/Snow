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
	cmdStr := ` 
		sudo iptables -A INPUT  -p tcp --dport {port} -j DROP &&
		sudo iptables -A OUTPUT -p tcp --dport {port} -j DROP &&
		sudo iptables -A INPUT  -p tcp --sport {port} -j DROP &&
		sudo iptables -A OUTPUT -p tcp --sport {port} -j DROP &&
		sudo iptables -A INPUT  -p udp --dport {port} -j DROP &&
		sudo iptables -A OUTPUT -p udp --dport {port} -j DROP &&
		sudo iptables -A INPUT  -p udp --sport {port} -j DROP &&
		sudo iptables -A OUTPUT -p udp --sport {port} -j DROP &&

		sudo iptables -A INPUT   -p tcp --sport {port} -m conntrack --ctstate ESTABLISHED -j DROP &&
		sudo iptables -A OUTPUT  -p tcp --sport {port} -m conntrack --ctstate ESTABLISHED -j DROP &&

		sudo iptables -A INPUT  -p tcp --dport {port} -m conntrack --ctstate ESTABLISHED -j DROP &&
		sudo iptables -A OUTPUT -p tcp --dport {port} -m conntrack --ctstate ESTABLISHED -j DROP &&

		sudo iptables -A INPUT   -p udp --sport {port} -m conntrack --ctstate ESTABLISHED -j DROP &&
		sudo iptables -A OUTPUT  -p udp --sport {port} -m conntrack --ctstate ESTABLISHED -j DROP &&

		sudo iptables -A INPUT  -p udp --dport {port} -m conntrack --ctstate ESTABLISHED -j DROP &&
		sudo iptables -A OUTPUT -p udp --dport {port} -m conntrack --ctstate ESTABLISHED -j DROP
	`
	cmd := strings.Replace(cmdStr, "{port}", portStr, -1)
	// 执行命令
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		fmt.Println("Error executing command:", err)
	}
	fmt.Println(string(out))
	err = ClearConntrackEntries(port)
	if err := ClearConntrackEntries(port); err != nil {
		return fmt.Errorf("failed to clear conntrack entries: %v", err)
	}
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
	//if CheckPortInUse(port + common.Offset) {
	err := DisablePort(port + common.Offset)
	if err != nil {
		panic(err)
	}
	//}

}
func ResetIPTable() error {
	cmdStr := "sudo iptables -F"
	out, err := exec.Command("bash", "-c", cmdStr).Output()
	if err != nil {
		fmt.Println("Error executing command:", err)
	}
	fmt.Println(string(out))
	return err
}
func DisableNodes(start int, end int) {
	for ; start < end; start++ {
		DisableNode(start)
	}
}
func ClearConntrackEntries(port int) error {
	portStr := strconv.Itoa(port)
	cmdStr :=
		"sudo conntrack -D -p tcp --src 127.0.0.1 --sport " + portStr + " && " +
			"sudo conntrack -D -p tcp --dst 127.0.0.1 --dport " + portStr + " && " +
			"sudo conntrack -D -p udp --dst 127.0.0.1 --dport " + portStr + " && " +
			"sudo conntrack -D -p udp --dst 127.0.0.1 --dport " + portStr

	out, err := exec.Command("bash", "-c", cmdStr).CombinedOutput()
	if err != nil {
		// 如果只是 "0 entries deleted"，不算错误
		if strings.Contains(string(out), "0 flow entries have been deleted") {
			return nil
		}
		return fmt.Errorf("failed to clear conntrack entries: %v\nOutput: %s", err, string(out))
	}
	fmt.Printf("Conntrack command output: %s\n", string(out))
	return nil
}
