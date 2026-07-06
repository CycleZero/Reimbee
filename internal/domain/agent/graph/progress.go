// Package graph Graph 定义层——构建 Eino compose.Graph 编译为 Runnable
// 进度查询子流程：ChatModel + query_progress 工具
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

// 进度查询子流程的系统提示词
const progressSystemPrompt = `你是 Reimbee，帮助用户查询报销进度。使用 query_progress 工具查询审批进度，使用 query_reimbursements 工具查询报销记录列表。
请根据用户提供的信息（如报销单号）进行查询，并以友好简洁的方式反馈进度结果。
如果用户没有提供报销单号，请先询问用户提供单号。`

// buildProgressGraph 构建进度查询子流程 Graph（未编译）
// 拓扑：
//
//	ChatModel 模式：START → build_prompt(Lambda) → progress_agent(ChatModel) → END
//	降级模式：   START → progress_agent(Lambda) → END
//
// 返回 *compose.Graph，由 Root Graph 通过 AddGraphNode 嵌套编译
func buildProgressGraph(logger *log.Logger, chatModel model.ToolCallingChatModel) *compose.Graph[agent.AgentInput, *schema.Message] {
	logger.Debug("开始构建进度查询子流程 Graph")

	g := compose.NewGraph[agent.AgentInput, *schema.Message]()

	if chatModel != nil {
		// ChatModel 模式：通过 Lambda 组装系统提示词 + 用户消息，再调用 ChatModel
		g.AddLambdaNode("build_prompt", compose.InvokableLambda(
			func(ctx context.Context, input agent.AgentInput) ([]*schema.Message, error) {
				logger.Debug("进度查询构建提示词", zap.String("用户消息", input.Message))
				return []*schema.Message{
					schema.SystemMessage(progressSystemPrompt),
					schema.UserMessage(input.Message),
				}, nil
			},
		))
		g.AddChatModelNode("progress_agent", chatModel)
		g.AddEdge(compose.START, "build_prompt")
		g.AddEdge("build_prompt", "progress_agent")
		g.AddEdge("progress_agent", compose.END)
	} else {
		// 降级模式：ChatModel 未配置，使用关键词匹配模板回复
		logger.Debug("ChatModel未配置，进度查询降级为模板回复")
		g.AddLambdaNode("progress_agent", compose.InvokableLambda(
			func(ctx context.Context, input agent.AgentInput) (*schema.Message, error) {
				logger.Debug("进度查询降级", zap.String("用户消息", input.Message))
				return schema.AssistantMessage(
					"您好！进度查询功能需要配置 AI 模型。请提供您的报销单号，或联系管理员启用 AI 功能。", nil,
				), nil
			},
		))
		g.AddEdge(compose.START, "progress_agent")
		g.AddEdge("progress_agent", compose.END)
	}

	logger.Info("进度查询子流程 Graph 构建完成")
	return g
}

// compileProgressGraph 编译进度查询子流程为 Runnable
// 用于独立编译场景（当前由 Root Graph 统一编译，此函数保留备用）
func compileProgressGraph(ctx context.Context, logger *log.Logger, chatModel model.ToolCallingChatModel) (compose.Runnable[agent.AgentInput, *schema.Message], error) {
	g := buildProgressGraph(logger, chatModel)
	runnable, err := g.Compile(ctx,
		compose.WithGraphName("query_progress_workflow"),
		compose.WithMaxRunSteps(10),
	)
	if err != nil {
		logger.Error("编译进度查询Graph失败", zap.Error(err))
		return nil, fmt.Errorf("编译进度查询Graph失败: %w", err)
	}
	logger.Info("进度查询子流程 Graph 编译成功")
	return runnable, nil
}
