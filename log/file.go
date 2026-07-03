package log

import (
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

func GetLogPath(logDir string, serviceName string) string {
	if logDir == "" {
		return ""
	}
	p := logDir +
		"/" +
		time.Now().Format("2006-01-02") +
		"/" +
		serviceName +
		"/" +
		time.Now().Format("2006-01-02-15-04-05") + ".log"
	return p
}

func NewFileWriter(logPath string) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename: logPath,
		MaxSize:  1, // megabytes
	}
}
