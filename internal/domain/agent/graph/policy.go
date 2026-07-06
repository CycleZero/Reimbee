// Package graph Graph 定义层——构建 Eino compose.Graph 编译为 Runnable
// 政策咨询子流程：ChatModel 纯文本回复（无工具调用），通过系统 Prompt 锚定政策知识
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

// policySystemPromptSuffix 政策咨询场景下追加到通用 Prompt 后的上下文提示
const policySystemPromptSuffix = `当前用户正在咨询报销政策。请根据公司报销政策知识库回答用户的问题，包括但不限于：
- 差旅标准（交通、住宿、餐饮补贴）
- 招待费限额
- 办公用品报销上限
- 特殊情况的报销流程

如遇到不确定的政策，请诚实地告知用户并建议其咨询财务部门。`

// buildPolicyGraph 构建政策咨询子流程 Graph（未编译）
// 拓扑：
//
//	ChatModel 模式：START → build_prompt(Lambda) → policy_agent(ChatModel) → END
//	降级模式：   START → policy_agent(Lambda) → END
//
// 返回 *compose.Graph，由 Root Graph 通过 AddGraphNode 嵌套编译
func buildPolicyGraph(logger *log.Logger, chatModel model.ToolCallingChatModel) *compose.Graph[agent.AgentInput, *schema.Message] {
	logger.Debug("开始构建政策咨询子流程 Graph")

	g := compose.NewGraph[agent.AgentInput, *schema.Message]()

	if chatModel != nil {
		// ChatModel 模式：使用 BuildGeneralChatPrompt + 政策上下文作为系统提示词
		g.AddLambdaNode("build_prompt", compose.InvokableLambda(
			func(ctx context.Context, input agent.AgentInput) ([]*schema.Message, error) {
				logger.Debug("政策咨询构建提示词", zap.String("用户消息", input.Message))
				basePrompt := agent.BuildGeneralChatPrompt()
				return []*schema.Message{
					schema.SystemMessage(basePrompt + "\n\n" + policySystemPromptSuffix),
					schema.UserMessage(input.Message),
				}, nil
			},
		))
		g.AddChatModelNode("policy_agent", chatModel)
		g.AddEdge(compose.START, "build_prompt")
		g.AddEdge("build_prompt", "policy_agent")
		g.AddEdge("policy_agent", compose.END)
	} else {
		// 降级模式：ChatModel 未配置，使用模板回复
		logger.Debug("ChatModel未配置，政策咨询降级为模板回复")
		g.AddLambdaNode("policy_agent", compose.InvokableLambda(
			func(ctx context.Context, input agent.AgentInput) (*schema.Message, error) {
				logger.Debug("政策咨询降级", zap.String("用户消息", input.Message))
				return schema.AssistantMessage(
					"您好！政策咨询功能需要配置 AI 模型。常见报销政策请参考公司内部知识库，或联系管理员启用 AI 功能。", nil,
				), nil
			},
		))
		g.AddEdge(compose.START, "policy_agent")
		g.AddEdge("policy_agent", compose.END)
	}

	logger.Info("政策咨询子流程 Graph 构建完成")
	return g
}

// compilePolicyGraph 编译政策咨询子流程为 Runnable（保留备用）
func compilePolicyGraph(ctx context.Context, logger *log.Logger, chatModel model.ToolCallingChatModel) (compose.Runnable[agent.AgentInput, *schema.Message], error) {
	g := buildPolicyGraph(logger, chatModel)
	runnable, err := g.Compile(ctx,
		compose.WithGraphName("policy_question_workflow"),
		compose.WithMaxRunSteps(5),
	)
	if err != nil {
		logger.Error("编译政策咨询Graph失败", zap.Error(err))
		return nil, fmt.Errorf("编译政策咨询Graph失败: %w", err)
	}
	logger.Info("政策咨询 Handler 编译成功")
	return runnable, nil
}
