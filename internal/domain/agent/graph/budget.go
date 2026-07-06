// Package graph Graph 定义层——构建 Eino compose.Graph 编译为 Runnable
// 预算查询子流程：ChatModel + check_budget 工具
package graph

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// 预算查询子流程的系统提示词
const budgetSystemPrompt = `你是 Reimbee，帮助用户查询部门预算。使用 check_budget 工具查询预算信息。
请根据用户的部门或项目查询相应预算余额、使用率及剩余金额，并以清晰的结构化方式展示结果。
如果用户未指定部门，请优先查询当前用户所属部门的预算情况。`

// buildBudgetGraph 构建预算查询子流程 Graph（未编译）
// 拓扑：
//
//	ChatModel 模式：START → build_prompt(Lambda) → budget_agent(ChatModel) → END
//	降级模式：   START → budget_agent(Lambda) → END
//
// 返回 *compose.Graph，由 Root Graph 通过 AddGraphNode 嵌套编译
func buildBudgetGraph(logger *log.Logger, chatModel model.ToolCallingChatModel) *compose.Graph[agent.AgentInput, *schema.Message] {
	logger.Debug("开始构建预算查询子流程 Graph")

	g := compose.NewGraph[agent.AgentInput, *schema.Message]()

	if chatModel != nil {
		// ChatModel 模式：通过 Lambda 组装系统提示词 + 用户消息，再调用 ChatModel
		g.AddLambdaNode("build_prompt", compose.InvokableLambda(
			func(ctx context.Context, input agent.AgentInput) ([]*schema.Message, error) {
				logger.Debug("预算查询构建提示词", zap.String("用户消息", input.Message))
				return []*schema.Message{
					schema.SystemMessage(budgetSystemPrompt),
					schema.UserMessage(input.Message),
				}, nil
			},
		))
		g.AddChatModelNode("budget_agent", chatModel)
		g.AddEdge(compose.START, "build_prompt")
		g.AddEdge("build_prompt", "budget_agent")
		g.AddEdge("budget_agent", compose.END)
	} else {
		// 降级模式：ChatModel 未配置，使用模板回复
		logger.Debug("ChatModel未配置，预算查询降级为模板回复")
		g.AddLambdaNode("budget_agent", compose.InvokableLambda(
			func(ctx context.Context, input agent.AgentInput) (*schema.Message, error) {
				logger.Debug("预算查询降级", zap.String("用户消息", input.Message))
				return schema.AssistantMessage(
					"您好！预算查询功能需要配置 AI 模型。请提供您的部门信息，或联系管理员启用 AI 功能。", nil,
				), nil
			},
		))
		g.AddEdge(compose.START, "budget_agent")
		g.AddEdge("budget_agent", compose.END)
	}

	logger.Info("预算查询子流程 Graph 构建完成")
	return g
}

// compileBudgetGraph 编译预算查询子流程为 Runnable（保留备用）
func compileBudgetGraph(ctx context.Context, logger *log.Logger, chatModel model.ToolCallingChatModel) (compose.Runnable[agent.AgentInput, *schema.Message], error) {
	g := buildBudgetGraph(logger, chatModel)
	runnable, err := g.Compile(ctx,
		compose.WithGraphName("query_budget_workflow"),
		compose.WithMaxRunSteps(10),
	)
	if err != nil {
		logger.Error("编译预算查询Graph失败", zap.Error(err))
		return nil, fmt.Errorf("编译预算查询Graph失败: %w", err)
	}
	logger.Info("预算查询子流程 Graph 编译成功")
	return runnable, nil
}
