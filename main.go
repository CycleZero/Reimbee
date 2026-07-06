package main

import (
	"github.com/CycleZero/Reimbee/conf"
	"github.com/CycleZero/Reimbee/log"
	"os"
	"os/signal"

	"go.uber.org/zap"
)

func main() {
	vc := conf.GetConfig()
	// 初始化日志（必须在 GetLogger 之前调用）
	if err := log.InitLogger(
		vc.GetString("log.mode"),
		vc.GetString("log.level"),
		vc.GetString("log.dir"),
	); err != nil {
		panic("日志初始化失败: " + err.Error())
	}
	logger := log.GetLogger()

	app := initApp(vc, logger)

	done := make(chan os.Signal, 1)
	go func() {
		defer func() {
			done <- os.Interrupt
		}()
		logger.Info("服务已启动")
		if err := app.StartServer(); err != nil {
			logger.Error("服务崩溃", zap.Error(err))
			return
		}
	}()

	signal.Notify(done, os.Interrupt)
	<-done
	logger.Info("服务退出")
}
