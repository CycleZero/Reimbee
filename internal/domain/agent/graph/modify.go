// Package graph Graph 定义层——构建 Eino compose.Graph 编译为 Runnable
// 修改报销子流程：ChatModel + query_progress 工具，用于修改已驳回的报销单
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

// 修改报销子流程的系统提示词
const modifySystemPrompt = `你是 Reimbee，帮助用户修改已驳回的报销单。使用 query_progress 工具查询原报销单信息和驳回原因，使用 query_reimbursements 工具查询可修改的报销记录。
请引导用户完成以下步骤：
1. 查询已被驳回的报销单
2. 展示驳回原因和原报销信息
3. 引导用户提供修改内容（金额、类别、票据等）
4. 确认后重新提交报销

请以耐心、清晰的方式引导用户完成整个修改流程。`

// buildModifyGraph 构建修改报销子流程 Graph（未编译）
// 拓扑：
//
//	ChatModel 模式：START → build_prompt(Lambda) → modify_agent(ChatModel) → END
//	降级模式：   START → modify_agent(Lambda) → END
//
// 返回 *compose.Graph，由 Root Graph 通过 AddGraphNode 嵌套编译
func buildModifyGraph(logger *log.Logger, chatModel model.ToolCallingChatModel) *compose.Graph[agent.AgentInput, *schema.Message] {
	logger.Debug("开始构建修改报销子流程 Graph")

	g := compose.NewGraph[agent.AgentInput, *schema.Message]()

	if chatModel != nil {
		// ChatModel 模式：通过 Lambda 组装系统提示词 + 用户消息，再调用 ChatModel
		g.AddLambdaNode("build_prompt", compose.InvokableLambda(
			func(ctx context.Context, input agent.AgentInput) ([]*schema.Message, error) {
				logger.Debug("修改报销构建提示词", zap.String("用户消息", input.Message))
				return []*schema.Message{
					schema.SystemMessage(modifySystemPrompt),
					schema.UserMessage(input.Message),
				}, nil
			},
		))
		g.AddChatModelNode("modify_agent", chatModel)
		g.AddEdge(compose.START, "build_prompt")
		g.AddEdge("build_prompt", "modify_agent")
		g.AddEdge("modify_agent", compose.END)
	} else {
		// 降级模式：ChatModel 未配置，使用模板回复
		logger.Debug("ChatModel未配置，修改报销降级为模板回复")
		g.AddLambdaNode("modify_agent", compose.InvokableLambda(
			func(ctx context.Context, input agent.AgentInput) (*schema.Message, error) {
				logger.Debug("修改报销降级", zap.String("用户消息", input.Message))
				return schema.AssistantMessage(
					"您好！修改报销功能需要配置 AI 模型。请提供被驳回报销单的单号，或联系管理员启用 AI 功能。", nil,
				), nil
			},
		))
		g.AddEdge(compose.START, "modify_agent")
		g.AddEdge("modify_agent", compose.END)
	}

	logger.Info("修改报销子流程 Graph 构建完成")
	return g
}

// compileModifyGraph 编译修改报销子流程为 Runnable（保留备用）
func compileModifyGraph(ctx context.Context, logger *log.Logger, chatModel model.ToolCallingChatModel) (compose.Runnable[agent.AgentInput, *schema.Message], error) {
	g := buildModifyGraph(logger, chatModel)
	runnable, err := g.Compile(ctx,
		compose.WithGraphName("modify_reimbursement_workflow"),
		compose.WithMaxRunSteps(10),
	)
	if err != nil {
		logger.Error("编译修改报销Graph失败", zap.Error(err))
		return nil, fmt.Errorf("编译修改报销Graph失败: %w", err)
	}
	logger.Info("修改报销子流程 Graph 编译成功")
	return runnable, nil
}
