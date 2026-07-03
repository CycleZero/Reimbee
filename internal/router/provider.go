package router

import (
	"github.com/CycleZero/Reimbee/internal/router/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"
)

var RouterProviderSet = wire.NewSet(
	NewRegisterFunc,
	NewRegisterMiddleWire,
)

// RegisteredMiddleWire 已注册的中间件集合
type RegisteredMiddleWire struct {
	JwtAuthMiddleWire func(optional bool) gin.HandlerFunc
}

// Register 完成中间件注册，必须在路由注册前调用
func (r *RegisteredMiddleWire) Register() {
	middleware.AuthMiddleWire = r.JwtAuthMiddleWire
	middleware.IsMiddleWireRegisterFinished = true
}

// NewRegisterMiddleWire 创建中间件注册器
// jwtSecret 可以从配置中读取，这里使用固定值作为示例
func NewRegisterMiddleWire() RegisteredMiddleWire {
	return RegisteredMiddleWire{
		JwtAuthMiddleWire: middleware.JwtAuthMiddleWire("your-jwt-secret-key-change-in-production"),
	}
}
