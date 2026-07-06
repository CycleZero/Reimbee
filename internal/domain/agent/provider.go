package agent

import (
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/google/wire"
)

// ProviderSet 智能体模块的 Wire 依赖注入集合
var ProviderSet = wire.NewSet(
	// Phase A — 配置加载
	LoadAgentConfig,

	// Phase B — 工具层
	tools.ProviderSet,

	// Phase D — ChatModel
	MustNewChatModel,

	// Phase D — Checkpoint 持久化
	NewMySQLCheckpointStore,
	wire.Bind(new(CheckpointStore), new(*MySQLCheckpointStore)),

	// Phase D — 运行引擎 + HTTP 服务
	NewAgentRunner,
	NewAgentService,
)
