package tool

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"time"
)

type ColorFormatter struct{}

func (f *ColorFormatter) Format(entry *log.Entry) ([]byte, error) {
	var levelColor int
	switch entry.Level {
	case log.DebugLevel:
		levelColor = 33 // 黄色
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
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,       // 完整时间
		TimestampFormat: "15:04:05", // 时间格式
		ForceColors:     false,      //显示颜色
	})
}

func InfoLog() {
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,       // 完整时间
		TimestampFormat: "15:04:05", // 时间格式
		ForceColors:     false,      //显示颜色
	})
}
