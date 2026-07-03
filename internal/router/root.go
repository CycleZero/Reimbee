package router

import (
	"github.com/CycleZero/Reimbee/internal/domain"
	"github.com/CycleZero/Reimbee/internal/router/middleware"

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

	// 健康检查
	root.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// 业务路由（后续在此注册各领域模块路由）
	api := root.Group("/api")
	_ = api // 占位，后续添加 reimbursement/budget/approval 路由
}
