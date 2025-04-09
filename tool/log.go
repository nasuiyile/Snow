package tool

import log "github.com/sirupsen/logrus"

func DebugLog() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,       // 完整时间
		TimestampFormat: "15:04:05", // 时间格式
		ForceColors:     true,       //显示颜色
	})
}
