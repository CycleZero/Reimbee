// Package agent 智能体层，负责基于多 Agent 编排的对话式报销流程管理
// 本文件定义 LoopManager 的 Phase Agent 初始化逻辑，
// 在服务启动时预创建全部 8 个 ChatModelAgent 实例
package agent

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"go.uber.org/zap"
)

// ============================================
// Agent 初始化 — Phase Agents + 通用 Agent + 子流程 Agents
// ============================================

// initAgents 初始化全部 8 个 ChatModelAgent 实例
// 由 NewLoopManager 在构造时调用，创建后存储在 LoopManager 的对应字段中
// 所有 Agent 在服务启动时预创建（非惰性），确保首次请求零延迟
//
// Agent 分类：
//   - Phase Agent ×3：报销三阶段流水线（collect → validate → execute）
//   - 通用 Agent ×1：问候、感谢、闲聊等非业务流程对话
//   - 子流程 Agent ×4：进度查询、预算查询、政策咨询、修改报销
func (m *LoopManager) initAgents(ctx context.Context, deps LoopManagerDeps) {
	deps.Logger.Debug("开始初始化全部Agent实例")

	// ============================================
	// Phase 1: 收集票据信息
	// ============================================
	m.phase1Agent = mustNewAgent(ctx, deps, "phase1_collect", "收集票据信息",
		BuildSystemPrompt("phase1_collect", nil),
		[]tool.BaseTool{deps.ToolSet.OCR, deps.ToolSet.Compliance, deps.ToolSet.ConfirmInvoice},
	)

	// ============================================
	// Phase 2: 合规与预算校验
	// ============================================
	m.phase2Agent = mustNewAgent(ctx, deps, "phase2_validate", "合规与预算校验",
		BuildSystemPrompt("phase2_validate", nil),
		[]tool.BaseTool{deps.ToolSet.Compliance, deps.ToolSet.Budget, deps.ToolSet.ConfirmSubmit},
	)

	// ============================================
	// Phase 3: 创建并提交报销单
	// ============================================
	m.phase3Agent = mustNewAgent(ctx, deps, "phase3_execute", "创建并提交报销单",
		BuildSystemPrompt("phase3_execute", nil),
		[]tool.BaseTool{
			deps.ToolSet.CreateReimb, deps.ToolSet.SubmitReimb,
			deps.ToolSet.PDF, deps.ToolSet.Email, deps.ToolSet.Progress,
		},
	)

	// ============================================
	// 通用对话 Agent（问候、感谢、闲聊）
	// ============================================
	m.chatAgent = mustNewAgent(ctx, deps, "general_chat", "通用对话",
		BuildGeneralChatPrompt(),
		nil, // 纯对话，无需工具
	)

	// ============================================
	// 子流程 Agent — 查询进度
	// ============================================
	m.progressAgent = mustNewAgent(ctx, deps, "query_progress", "查询进度",
		"你是 Reimbee，帮助用户查询报销进度。使用 query_progress 工具查询审批进度，"+
			"使用 query_reimbursements 工具查询报销记录列表。"+
			"请根据用户提供的信息进行查询，并以友好简洁的方式反馈进度结果。",
		[]tool.BaseTool{deps.ToolSet.Progress, deps.ToolSet.QueryRecords},
	)

	// ============================================
	// 子流程 Agent — 查询预算
	// ============================================
	m.budgetAgent = mustNewAgent(ctx, deps, "query_budget", "查询预算",
		"你是 Reimbee，帮助用户查询部门预算。使用 check_budget 工具查询预算信息。"+
			"请根据用户的部门或项目查询相应预算余额、使用率及剩余金额，并以清晰的结构化方式展示结果。",
		[]tool.BaseTool{deps.ToolSet.Budget},
	)

	// ============================================
	// 子流程 Agent — 政策咨询
	// ============================================
	m.policyAgent = mustNewAgent(ctx, deps, "policy_question", "政策咨询",
		BuildGeneralChatPrompt(),
		nil, // 纯知识问答，无需工具
	)

	// ============================================
	// 子流程 Agent — 修改报销
	// ============================================
	m.modifyAgent = mustNewAgent(ctx, deps, "modify_reimbursement", "修改报销",
		"你是 Reimbee，帮助用户修改已有的报销单。使用 query_reimbursements 工具查看用户的历史报销记录及其状态。"+
			"根据用户需求，协助修改被驳回的报销单。",
		[]tool.BaseTool{deps.ToolSet.Progress, deps.ToolSet.QueryRecords},
	)

	deps.Logger.Info("全部Agent初始化完成",
		zap.Int("Phase_Agent数", 3),
		zap.Int("通用Agent数", 1),
		zap.Int("子流程Agent数", 4),
		zap.Int("总计", 8),
	)
}

// ============================================
// mustNewAgent — Agent 工厂函数
// ============================================

// mustNewAgent 创建 ChatModelAgent 实例，失败时 panic
// 用于服务启动阶段的 Agent 初始化，任何创建失败均为致命错误（无法降级）
//
// 参数：
//   - ctx: 上下文（传递至 Eino adk 框架）
//   - deps: 依赖注入结构（包含 ChatModel、Logger 等共享组件）
//   - name: Agent 唯一标识名（如 "phase1_collect"）
//   - desc: Agent 功能描述（用于日志和调试）
//   - instruction: 系统指令（Prompt），注入到每轮对话的 SystemMessage 中
//   - toolList: 工具列表，nil 表示纯对话 Agent（无工具调用能力）
//
// 返回：
//   - *adk.ChatModelAgent: 已初始化的 Agent 实例，内置 ReAct 循环
//
// 配置说明：
//   - MaxIterations=10：防止死循环，Agent 最多执行 10 轮 ReAct（思考→工具调用→思考）
//   - ToolsConfig：通过 compose.ToolsNodeConfig 传递工具列表，Eino 自动管理工具调用和结果注入
func mustNewAgent(ctx context.Context, deps LoopManagerDeps,
	name, desc, instruction string, toolList []tool.BaseTool) *adk.ChatModelAgent {

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        name,
		Description: desc,
		Instruction: instruction,
		Model:       deps.ChatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: toolList},
		},
		MaxIterations: 10, // 防止死循环：最多10轮ReAct循环
	})
	if err != nil {
		deps.Logger.Error("创建Agent失败", zap.String("name", name), zap.Error(err))
		panic("创建Agent失败: " + name + ": " + err.Error())
	}
	deps.Logger.Debug("Agent创建成功", zap.String("name", name), zap.Int("工具数", len(toolList)))
	return agent
}
