// Package agent 智能体层
// 本文件定义 LoopManager 的 Agent 初始化逻辑，v4 创建唯一 ReimburseAgent 实例
package agent

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"go.uber.org/zap"
)

// initAgent 创建唯一的 ReimburseAgent 实例，持有全部 9 个工具
func (m *LoopManager) initAgent(ctx context.Context, deps LoopManagerDeps) {
	deps.Logger.Info("初始化 ReimburseAgent（v4 单Agent模式）")

	m.msgCollector = &messageCollector{}
	mw := &collectorMiddleware{collector: m.msgCollector}

	m.reimburseAgent = mustNewAgent(ctx, deps,
		"reimburse_agent",
		"企业报销全流程智能助手",
		BuildSystemPromptV4(),
		[]tool.BaseTool{
			deps.ToolSet.OCR,
			deps.ToolSet.Compliance,
			deps.ToolSet.Budget,
			deps.ToolSet.CreateReimb,
			deps.ToolSet.SubmitReimb,
			deps.ToolSet.PDF,
			deps.ToolSet.Email,
			deps.ToolSet.Progress,
			deps.ToolSet.QueryRecords,
		},
		[]adk.ChatModelAgentMiddleware{mw},
	)

	deps.Logger.Info("ReimburseAgent初始化完成", zap.Int("工具数", 9))
}

// mustNewAgent 创建 ChatModelAgent 实例，失败时 panic
func mustNewAgent(ctx context.Context, deps LoopManagerDeps,
	name, desc, instruction string, toolList []tool.BaseTool,
	handlers []adk.ChatModelAgentMiddleware) *adk.ChatModelAgent {

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        name,
		Description: desc,
		Instruction: instruction,
		Model:       deps.ChatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: toolList},
		},
		MaxIterations: 15,
		Handlers:      handlers,
	})
	if err != nil {
		deps.Logger.Error("创建Agent失败", zap.String("name", name), zap.Error(err))
		panic("创建Agent失败: " + name + ": " + err.Error())
	}
	deps.Logger.Debug("Agent创建成功", zap.String("name", name), zap.Int("工具数", len(toolList)))
	return agent
}
