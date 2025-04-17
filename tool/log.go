package tool

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"time"
)

type ColorFormatter struct{}

func (f *ColorFormatter) Format(entry *log.Entry) ([]byte, error) {
	var levelColor int
	switch entry.Level {
	case log.DebugLevel:
		levelColor = 37 // 黄色
	case log.InfoLevel:
		levelColor = 36 // 青色
	case log.WarnLevel:
		levelColor = 35 // 紫色
	case log.ErrorLevel, log.FatalLevel, log.PanicLevel:
		levelColor = 31 // 红色
	default:
		levelColor = 37 // 默认白色
	}

	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf("\x1b[%dm[%s] [%s] %s\x1b[0m\n", levelColor, timestamp, entry.Level.String(), entry.Message)

	return []byte(msg), nil
}

func DebugLog() {
	logFile, err := os.OpenFile("Snow.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("无法打开日志文件:", err)
	}

	// 创建一个同时输出到控制台和文件的 writer
	multiWriter := io.MultiWriter(os.Stdout, logFile)

	// 设置标准日志输出
	log.SetOutput(multiWriter)

	// 关键操作：将 os.Stderr 重定向到 multiWriter
	os.Stderr = logFile // 如果你不需要控制台输出，只写到文件就这样写

	// 如果要同时输出到控制台和文件，要这么做：
	// os.Stderr = multiWriter（这在某些系统上不生效，可用 dup2 系统调用实现）

	// 示例日志
	log.Println("程序开始运行")

	// 测试输出
	log.Println("这是一个日志输出示例")

	log.SetFormatter(&ColorFormatter{})
	log.SetLevel(log.InfoLevel)
}
