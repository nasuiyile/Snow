package main

import (
	"flag"
	"fmt"
	"os/exec"
	"sync/atomic"
)

var counter = atomic.Int32{}

func main() {
	port := 0
	latency := 0
	jitter := 0
	flag.IntVar(&port, "port", 8100, "Port to apply latency")
	flag.IntVar(&latency, "latency", 500, "Latency")
	flag.IntVar(&jitter, "jitter", 500, "jitter")
	flag.Parse()
	InitRule()
	AddLatency(port, latency, jitter) // 添加延迟规则，端口范围从20000到20004
}

func InitRule() {
	clean := "sudo tc qdisc del dev lo root"
	err := exec.Command("bash", "-c", clean).Run()
	if err != nil {
		//不影响执行
		println(err.Error())
	}
	qdisc := "sudo tc qdisc replace dev lo root handle 1: prio"
	err = exec.Command("bash", "-c", qdisc).Run()
	if err != nil {
		println(err.Error())
	}
}
func AddLatency(port int, latency int, jitter int) {
	add := counter.Add(1)
	setLatency := fmt.Sprintf("sudo tc qdisc replace dev lo parent 1:%d handle 10: netem delay %dus %dus distribution normal limit 1048576", add, latency, jitter)
	fmt.Println(setLatency)
	err := exec.Command("bash", "-c", setLatency).Run()
	if err != nil {
		println("Failed to replace latency:", err.Error())
		return
	}
	setFilter := fmt.Sprintf("sudo tc filter replace dev lo protocol ip parent 1:0 prio 1 u32 match ip dport %d 0xffff flowid 1:%d", port, add)
	fmt.Println(setFilter)
	err = exec.Command("bash", "-c", setFilter).Run()
}
