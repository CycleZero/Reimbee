package graph

import (
	"context"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/wire"
	"go.uber.org/zap"
)

// ProviderSet Graph 层的 Wire 依赖注入集合
var ProviderSet = wire.NewSet(
	NewRootGraphRunnable,
)

// RootGraphRunnable 包装 compose.Runnable 以便 Wire 类型区分
type RootGraphRunnable struct {
	compose.Runnable[agent.AgentInput, *schema.Message]
}

// NewRootGraphRunnable 构建编译后的 Root Graph Runnable
// chatModel：LLM 实例（用于意图分类 + 通用对话 + 子流程），可为 nil 时降级为关键词匹配
func NewRootGraphRunnable(logger *log.Logger, chatModel model.ToolCallingChatModel) *RootGraphRunnable {
	logger.Debug("开始构建 Root Graph")

	// 构建各子流程 Graph（未编译，由 Root Graph 统一编译）
	progressGraph := buildProgressGraph(logger, chatModel)
	budgetGraph := buildBudgetGraph(logger, chatModel)
	policyGraph := buildPolicyGraph(logger, chatModel)
	modifyGraph := buildModifyGraph(logger, chatModel)

	deps := RootGraphDeps{
		Logger:                logger,
		ChatModel:             chatModel,
		ReimbursementRunnable: nil, // Phase D 后续接入
		ProgressGraph:         progressGraph,
		BudgetGraph:           budgetGraph,
		PolicyGraph:           policyGraph,
		ModifyGraph:           modifyGraph,
	}

	ctx := context.Background()
	runnable, err := NewRootGraph(ctx, deps)
	if err != nil {
		logger.Error("编译Root Graph失败", zap.Error(err))
		panic("编译Root Graph失败: " + err.Error())
	}

	mode := "关键词匹配降级模式"
	if chatModel != nil {
		mode = "ChatModel意图分类模式"
	}
	logger.Info("Root Graph 编译成功（" + mode + "）")
	return &RootGraphRunnable{runnable}
}
