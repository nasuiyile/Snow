package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	// 指定要读取的文件路径
	filePath := "C:\\Users\\12968\\Downloads\\pinglog\\2.log" // 替换为你的文件路径
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("打开文件时出错:", err)
	}
	defer file.Close() // 确保文件在函数结束时关闭
	// 创建一个新的Scanner来读取文件
	scanner := bufio.NewScanner(file)
	// 逐行扫描文件
	lineNumber := 1

	scanner.Scan()
	text := scanner.Text()
	IpHandler(strings.Split(text, " "))
	for scanner.Scan() {
		// 获取当前行的内容
		line := scanner.Text()
		split := strings.Split(line, "  ")
		LatencyHandler(split)
		lineNumber++
	}
	// 检查扫描过程中是否有错误
	if err := scanner.Err(); err != nil {
		fmt.Println("读取文件时出错:", err)
		return
	}
	fmt.Println("文件读取完成")
}

var IpList = make(map[int]string, 0)
var LatencyMap = make(map[string][]float64)

func IpHandler(line []string) {
	for i, v := range line {
		IpList[i] = v
		LatencyMap[v] = make([]float64, 0)
	}
}

func LatencyHandler(line []string) {
	for i, v := range line {
		if v == "" {
			continue
		}
		if i%2 == 1 {
			continue
		}
		index := i / 2
		ip := IpList[index]
		float, err := strconv.ParseFloat(v[:len(v)-2], 64)
		if err != nil {
			fmt.Println("转换为浮点数时出错:", err)
			continue
		}
		LatencyMap[ip] = append(LatencyMap[ip], float)
	}
}
