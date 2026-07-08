package compliance

import "github.com/google/wire"

// ProviderSet 合规检查模块的 Wire 依赖注入集合
var ProviderSet = wire.NewSet(
	NewKnowledgeBase,
	NewComplianceBiz,
	NewComplianceService,
)
