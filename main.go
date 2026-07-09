package main

import (
	"flag"

	"github.com/CycleZero/Reimbee/conf"
	"github.com/CycleZero/Reimbee/log"
	"os"
	"os/signal"

	"go.uber.org/zap"
)

func main() {
	seed := flag.Bool("seed", false, "预置演示数据（部门、员工、预算、报销单）")
	flag.Parse()

	vc := conf.GetConfig()
	if err := log.InitLogger(
		vc.GetString("log.mode"),
		vc.GetString("log.level"),
		vc.GetString("log.dir"),
	); err != nil {
		panic("日志初始化失败: " + err.Error())
	}
	logger := log.GetLogger()

	app := initApp(vc, logger)

	if *seed {
		logger.Info("开始预置演示数据...")
		if err := SeedDemoData(app.data.DB); err != nil {
			logger.Error("预置数据失败", zap.Error(err))
			os.Exit(1)
		}
		logger.Info("演示数据预置完成")
	}

	done := make(chan os.Signal, 1)
	go func() {
		defer func() { done <- os.Interrupt }()
		logger.Info("服务已启动")
		if err := app.StartServer(); err != nil {
			logger.Error("服务崩溃", zap.Error(err))
		}
	}()

	signal.Notify(done, os.Interrupt)
	<-done
	logger.Info("服务退出")
}
