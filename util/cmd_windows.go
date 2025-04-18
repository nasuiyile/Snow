package util

import (
	"fmt"
	"net"
	"os/exec"
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
	//portStr := strconv.Itoa(port)
	//
	//// PowerShell 命令：添加入站 & 出站的 TCP 和 UDP 封锁规则
	//cmdStr := fmt.Sprintf(`
	//	New-NetFirewallRule -DisplayName "Block_TCP_%[1]s_In" -Direction Inbound -Protocol TCP -LocalPort %[1]s -Action Block;
	//	New-NetFirewallRule -DisplayName "Block_TCP_%[1]s_Out" -Direction Outbound -Protocol TCP -LocalPort %[1]s -Action Block;
	//	New-NetFirewallRule -DisplayName "Block_UDP_%[1]s_In" -Direction Inbound -Protocol UDP -LocalPort %[1]s -Action Block;
	//	New-NetFirewallRule -DisplayName "Block_UDP_%[1]s_Out" -Direction Outbound -Protocol UDP -LocalPort %[1]s -Action Block;
	//`, portStr)
	//
	//// 调用 powershell 执行命令（需要管理员权限）
	//cmd := exec.Command("powershell", "-Command", cmdStr)
	//out, err := cmd.CombinedOutput()
	//if err != nil {
	//	fmt.Println("执行 PowerShell 命令出错:", err)
	//	fmt.Println("输出信息:", string(out))
	//	return err
	//}
	//fmt.Println("防火墙规则添加成功：", string(out))
	//
	//// Windows 上一般不需要 conntrack（Linux 专有），可略去
	return nil
}

func DisableNode(port int) {
	//// 关闭服务器和客户端的端口
	//err := DisablePort(port)
	//if err != nil {
	//	panic(err)
	//}
	//err = DisablePort(port + common.Offset)
	//if err != nil {
	//	panic(err)
	//}

}
func ResetIPTable() error {
	//// PowerShell 命令：删除所有用户添加的防火墙规则
	//cmdStr := `Get-NetFirewallRule | Where-Object {$_.Group -eq ""} | Remove-NetFirewallRule`
	//
	//cmd := exec.Command("powershell", "-Command", cmdStr)
	//out, err := cmd.CombinedOutput()
	//if err != nil {
	//	fmt.Println("执行 PowerShell 命令出错:", err)
	//	fmt.Println("输出信息:", string(out))
	//	return err
	//}
	//fmt.Println("防火墙规则已清除：", string(out))
	return nil
}
func DisableNodes(start int, end int) {
	for ; start < end; start++ {
		DisableNode(start)
	}
}
func ClearConntrackEntries(port int) error {
	portStr := strconv.Itoa(port)
	cmdStr := "sudo conntrack -D -p tcp --src 127.0.0.1 --sport " + portStr + " && " +
		"sudo conntrack -D -p tcp --dst 127.0.0.1 --dport " + portStr

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
