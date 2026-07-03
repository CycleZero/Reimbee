package router

import (
	"gin-template/internal/domain"
	"gin-template/internal/router/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterFunc 路由注册函数类型
type RegisterFunc func(root gin.IRouter, serviceHub *domain.ServiceHub)

// NewRegisterFunc 创建路由注册函数
func NewRegisterFunc() RegisterFunc {
	return RegisterRouter
}

// RegisterRouter 注册所有路由
func RegisterRouter(root gin.IRouter, serviceHub *domain.ServiceHub) {
	if !middleware.IsMiddleWireRegisterFinished {
		panic("中间件注册未完成")
	}

	// 全局中间件
	root.Use(middleware.CORS())
	root.Use(middleware.AddMetaData())

	// 注册业务路由
	api := root.Group("/api")
	RegisterDemoRouter(api, serviceHub)
}

// RegisterDemoRouter 注册 Demo 模块路由
func RegisterDemoRouter(api gin.IRouter, hub *domain.ServiceHub) {
	demo := api.Group("/demo")
	{
		demo.POST("", hub.DemoService.Create)
		demo.GET("", hub.DemoService.List)
		demo.GET("/:id", hub.DemoService.GetByID)
		demo.PUT("/:id", hub.DemoService.Update)
		demo.DELETE("/:id", hub.DemoService.Delete)
	}
}
