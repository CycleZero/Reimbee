package auth

import "github.com/google/wire"

// ProviderSet 认证模块的 Wire 依赖注入集合
var ProviderSet = wire.NewSet(
	NewEmployeeRepo,
	NewAuthBiz,
	NewAuthService,
)
