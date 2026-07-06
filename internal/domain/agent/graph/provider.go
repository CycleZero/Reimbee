package graph

import (
	"context"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/wire"
	"go.uber.org/zap"
)

var ProviderSet = wire.NewSet(
	NewRootGraphRunnable,
)

type RootGraphRunnable struct {
	compose.Runnable[agent.AgentInput, *schema.Message]
}

func NewRootGraphRunnable(
	logger *log.Logger,
	chatModel model.ToolCallingChatModel,
	toolSet *tools.ToolSet,
	config *agent.AgentConfig,
) *RootGraphRunnable {
	logger.Debug("开始构建 Root Graph（含全部子流程）")

	ctx := context.Background()

	// 报销子流程（三阶段 ChatModelAgent，独立编译）
	reimbDeps := ReimbursementGraphDeps{
		Logger:    logger,
		ToolSet:   toolSet,
		ChatModel: chatModel,
		Config:    config,
	}
	reimbRunnable, err := NewReimbursementGraph(ctx, reimbDeps)
	if err != nil {
		logger.Error("编译报销子流程失败", zap.Error(err))
		panic("编译报销子流程失败: " + err.Error())
	}

	progressGraph := buildProgressGraph(logger, chatModel)
	budgetGraph := buildBudgetGraph(logger, chatModel)
	policyGraph := buildPolicyGraph(logger, chatModel)
	modifyGraph := buildModifyGraph(logger, chatModel)

	deps := RootGraphDeps{
		Logger:                  logger,
		ChatModel:               chatModel,
		ReimbursementRunnable:   reimbRunnable,
		ProgressGraph:           progressGraph,
		BudgetGraph:             budgetGraph,
		PolicyGraph:             policyGraph,
		ModifyGraph:             modifyGraph,
	}

	runnable, err := NewRootGraph(ctx, deps)
	if err != nil {
		logger.Error("编译Root Graph失败", zap.Error(err))
		panic("编译Root Graph失败: " + err.Error())
	}

	logger.Info("Root Graph 编译成功（全子流程已挂载）")
	return &RootGraphRunnable{runnable}
}
