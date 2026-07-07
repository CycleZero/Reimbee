package main

import (
	"net/http"
	"strconv"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain"
	"github.com/CycleZero/Reimbee/internal/router"
	"github.com/CycleZero/Reimbee/log"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// MainApp 应用主结构，封装 Gin Engine 和所有基础设施
type MainApp struct {
	Engine       *gin.Engine
	ServiceHub   *domain.ServiceHub
	port         uint
	host         string
	data         *infra.Data
	RegisterFunc router.RegisterFunc
}

// NewMainApp 创建主应用实例（由 Wire 注入）
func NewMainApp(
	vc *viper.Viper,
	hub *domain.ServiceHub,
	registerFunc router.RegisterFunc,
	registeredMiddleWire router.RegisteredMiddleWire,
	data *infra.Data,
) *MainApp {
	gin.SetMode(gin.DebugMode)

	e := gin.New()

	// 基础中间件
	e.Use(gin.Logger())
	e.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		log.SugaredLogger().Errorf("发生 Panic: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器内部错误(panic)"})
	}))

	// 注册自定义中间件（必须在路由注册前完成）
	registeredMiddleWire.Register()

	// 注册 pprof（性能分析）
	pprof.Register(e)

	// 注册业务路由
	registerFunc(e, hub)

	app := &MainApp{
		Engine:       e,
		port:         vc.GetUint("server.http.port"),
		host:         vc.GetString("server.http.host"),
		ServiceHub:   hub,
		RegisterFunc: registerFunc,
	}

	app.printRoutes()
	return app
}

func (a *MainApp) printRoutes() {
	routes := a.Engine.Routes()
	log.SugaredLogger().Infof("Total routes: %d", len(routes))
	for _, route := range routes {
		log.SugaredLogger().Infof("  %-6s %s", route.Method, route.Path)
	}
}

// StartServer 启动 HTTP 服务
func (a *MainApp) StartServer() error {
	addr := a.host + ":" + strconv.FormatUint(uint64(a.port), 10)
	log.GetLogger().Info("启动服务 " + addr)
	return a.Engine.Run(addr)
}

// Close 关闭应用，释放资源
func (a *MainApp) Close() error {
	return nil
}
