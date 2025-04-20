package util

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

type ColorFormatter struct{}

func (f *ColorFormatter) Format(entry *log.Entry) ([]byte, error) {
	var levelColor int
	switch entry.Level {
	case log.DebugLevel:
		levelColor = 90 // 黄色
	case log.InfoLevel:
		levelColor = 36 // 青色
	case log.WarnLevel:
		levelColor = 37 // 紫色
	case log.ErrorLevel, log.FatalLevel, log.PanicLevel:
		levelColor = 31 // 红色
	default:
		levelColor = 37 // 默认白色
	}
	timestamp := time.Now().Format("15:04:05")
	// 给 level 加颜色，其他不变
	coloredLevel := fmt.Sprintf("\x1b[%dm%s\x1b[0m", levelColor, strings.ToUpper(entry.Level.String()))

	// 构建最终格式： [时间] [LEVEL] 消息
	msg := fmt.Sprintf("[%s] [%s] %s\n", timestamp, coloredLevel, entry.Message)

	return []byte(msg), nil
}

func DebugLog() {
	// 示例日志
	log.Println("程序开始运行")

	// 测试输出
	log.Println("这是一个日志输出示例")

	log.SetFormatter(&ColorFormatter{})
	log.SetLevel(log.InfoLevel)
}
