package graph

import (
	"context"
	"fmt"

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

	// 进度/预算/政策/修改子流程——编译为 Runnable
	progressRunnable := mustCompileGraph(ctx, logger, "进度查询", buildProgressGraph(logger, chatModel))
	budgetRunnable := mustCompileGraph(ctx, logger, "预算查询", buildBudgetGraph(logger, chatModel))
	policyRunnable := mustCompileGraph(ctx, logger, "政策咨询", buildPolicyGraph(logger, chatModel))
	modifyRunnable := mustCompileGraph(ctx, logger, "修改报销", buildModifyGraph(logger, chatModel))

	deps := RootGraphDeps{
		Logger:                logger,
		ChatModel:             chatModel,
		Config:                config,
		ReimbursementRunnable: reimbRunnable,
		ProgressRunnable:      progressRunnable,
		BudgetRunnable:        budgetRunnable,
		PolicyRunnable:        policyRunnable,
		ModifyRunnable:        modifyRunnable,
	}

	runnable, err := NewRootGraph(ctx, deps)
	if err != nil {
		logger.Error("编译Root Graph失败", zap.Error(err))
		panic("编译Root Graph失败: " + err.Error())
	}

	logger.Info("Root Graph 编译成功（全子流程已挂载）")
	return &RootGraphRunnable{runnable}
}

func mustCompileGraph(
	ctx context.Context,
	logger *log.Logger,
	name string,
	g *compose.Graph[agent.AgentInput, *schema.Message],
) compose.Runnable[*schema.Message, *schema.Message] {
	r, err := g.Compile(ctx, compose.WithGraphName(name), compose.WithMaxRunSteps(20))
	if err != nil {
		logger.Error("编译"+name+"子流程失败", zap.Error(err))
		panic("编译" + name + "子流程失败: " + err.Error())
	}
	// Wrap: AgentInput → *schema.Message
	return &agentInputAdapter{r}
}

// agentInputAdapter 适配 compose.Runnable[AgentInput, *schema.Message] → compose.Runnable[*schema.Message, *schema.Message]
type agentInputAdapter struct {
	inner compose.Runnable[agent.AgentInput, *schema.Message]
}

func (a *agentInputAdapter) Invoke(ctx context.Context, input *schema.Message, opts ...compose.Option) (*schema.Message, error) {
	ai := agent.AgentInput{Message: input.Content}
	if uc, ok := ctx.Value(userContextKey{}).(agent.AgentInput); ok {
		ai = uc
		ai.Message = input.Content
	}
	return a.inner.Invoke(ctx, ai, opts...)
}

func (a *agentInputAdapter) Stream(ctx context.Context, input *schema.Message, opts ...compose.Option) (*schema.StreamReader[*schema.Message], error) {
	ai := agent.AgentInput{Message: input.Content}
	if uc, ok := ctx.Value(userContextKey{}).(agent.AgentInput); ok {
		ai = uc
		ai.Message = input.Content
	}
	return a.inner.Stream(ctx, ai, opts...)
}

func (a *agentInputAdapter) Collect(ctx context.Context, input *schema.StreamReader[*schema.Message], opts ...compose.Option) (*schema.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *agentInputAdapter) Transform(ctx context.Context, input *schema.StreamReader[*schema.Message], opts ...compose.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("not implemented")
}
