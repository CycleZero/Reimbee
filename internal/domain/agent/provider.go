package agent

import (
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/google/wire"
)

// ProviderSet 智能体模块的 Wire 依赖注入集合
var ProviderSet = wire.NewSet(
	// 配置加载
	LoadAgentConfig,
	LoadLoopConfig,

	// 工具层
	tools.ProviderSet,

	// ChatModel
	MustNewChatModel,

	// Checkpoint 持久化（Eino 只提供接口，我们提供 MySQL 实现）
	NewMySQLCheckpointStore,
	wire.Bind(new(CheckpointStore), new(*MySQLCheckpointStore)),

	// v3.0: LoopManager + HTTP 服务
	NewLoopManager,
	NewAgentService,
)
