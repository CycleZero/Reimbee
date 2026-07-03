package main

import (
	"gin-template/conf"
	"gin-template/log"
	"os"
	"os/signal"

	"go.uber.org/zap"
)

func main() {
	vc := conf.GetConfig()
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
