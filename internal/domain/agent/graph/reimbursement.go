package graph

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/internal/domain/agent/phase"
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/compose"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ============================================
// 报销子流程 Graph 构建
// ============================================

// ReimbursementGraphDeps 报销子流程 Graph 构建所需的依赖
type ReimbursementGraphDeps struct {
	Logger    *log.Logger
	ToolSet   *tools.ToolSet
	ChatModel einoModel.ToolCallingChatModel
	Config    *agent.AgentConfig
}

// NewReimbursementGraph 构建报销三阶段子流程 Graph
// Graph 类型：[*schema.Message, *schema.Message] — 消息进，消息出
// 状态管理：通过 compose.WithGenLocalState[*agent.ReimbursementState] 管理共享状态
// 拓扑：START → Phase1 → Guard1 → [Branch] → Phase2 → Guard2 → [Branch] → Phase3 → END
// 阶段间通过 Branch + Guard 实现条件跳转（不满足则回退当前阶段）
func NewReimbursementGraph(ctx context.Context, deps ReimbursementGraphDeps) (compose.Runnable[*schema.Message, *schema.Message], error) {
	deps.Logger.Debug("开始构建报销子流程 Graph")

	g := compose.NewGraph[*schema.Message, *schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *agent.ReimbursementState {
			return &agent.ReimbursementState{CurrentPhase: "phase1_collect"}
		}),
	)

	// ========================
	// Phase 1: 信息收集
	// ========================
	phase1Prompt := agent.BuildSystemPrompt("phase1_collect", nil)
	var phase1ToolInfos []*schema.ToolInfo
	if deps.ToolSet != nil {
		phase1ToolInfos = toolsToInfos(ctx, deps.ToolSet.GetPhase1Tools())
	}
	g.AddLambdaNode("phase1_collect", newPhaseNode(deps.ChatModel, phase1Prompt, phase1ToolInfos, deps.Logger, "phase1_collect"))

	// Guard 1: 检查 Phase 1 退出条件
	g.AddLambdaNode("phase1_guard", compose.InvokableLambda(checkPhase1Guard))

	// Guard 1 分支：通过 → Phase 2，未通过 → 返回 Phase 1
	g.AddBranch("phase1_guard", compose.NewGraphBranch(
		guard1Router,
		map[string]bool{"phase1_collect": true, "phase2_validate": true},
	))

	// ========================
	// Phase 2: 校验确认
	// ========================
	phase2Prompt := agent.BuildSystemPrompt("phase2_validate", nil)
	var phase2ToolInfos []*schema.ToolInfo
	if deps.ToolSet != nil {
		phase2ToolInfos = toolsToInfos(ctx, deps.ToolSet.GetPhase2Tools())
	}
	g.AddLambdaNode("phase2_validate", newPhaseNode(deps.ChatModel, phase2Prompt, phase2ToolInfos, deps.Logger, "phase2_validate"))

	// Guard 2: 检查 Phase 2 退出条件
	g.AddLambdaNode("phase2_guard", compose.InvokableLambda(checkPhase2Guard))

	// Guard 2 分支：通过 → Phase 3，未通过 → 返回 Phase 2
	g.AddBranch("phase2_guard", compose.NewGraphBranch(
		guard2Router,
		map[string]bool{"phase2_validate": true, "phase3_execute": true},
	))

	// ========================
	// Phase 3: 执行提交
	// ========================
	phase3Prompt := agent.BuildSystemPrompt("phase3_execute", nil)
	var phase3ToolInfos []*schema.ToolInfo
	if deps.ToolSet != nil {
		phase3ToolInfos = toolsToInfos(ctx, deps.ToolSet.GetPhase3Tools())
	}
	g.AddLambdaNode("phase3_execute", newPhaseNode(deps.ChatModel, phase3Prompt, phase3ToolInfos, deps.Logger, "phase3_execute"))

	// ========================
	// 边连接
	// ========================
	g.AddEdge(compose.START, "phase1_collect")
	g.AddEdge("phase1_collect", "phase1_guard")
	// phase1_guard → phase2 或 phase1（由分支决定）
	g.AddEdge("phase2_validate", "phase2_guard")
	// phase2_guard → phase3 或 phase2（由分支决定）
	g.AddEdge("phase3_execute", compose.END)

	// ========================
	// 编译 Graph
	// ========================
	runnable, err := g.Compile(ctx,
		compose.WithGraphName("reimbursement_workflow"),
		compose.WithMaxRunSteps(50),
	)
	if err != nil {
		deps.Logger.Error("编译报销子流程Graph失败", zap.Error(err))
		return nil, fmt.Errorf("编译报销子流程Graph失败: %w", err)
	}

	deps.Logger.Info("报销子流程 Graph 编译成功")
	return runnable, nil
}

// ============================================
// Phase 节点工厂
// ============================================

// newPhaseNode 创建调用 ChatModel 的阶段 Lambda 节点
// 每次用户消息进入阶段时，将 system prompt + 用户消息 + 阶段工具配置发给 LLM
// 返回 LLM 的助理回复消息
func newPhaseNode(
	chatModel einoModel.ToolCallingChatModel,
	systemPrompt string,
	toolInfos []*schema.ToolInfo,
	logger *log.Logger,
	phaseName string,
) *compose.Lambda {
	return compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) (*schema.Message, error) {
		// 构建消息列表：系统提示 + 用户消息
		messages := []*schema.Message{
			schema.SystemMessage(systemPrompt),
			msg,
		}

		// 调用 LLM（带工具配置）
		response, err := chatModel.Generate(ctx, messages, einoModel.WithTools(toolInfos))
		if err != nil {
			logger.Error("LLM调用失败", zap.String("阶段", phaseName), zap.Error(err))
			return schema.AssistantMessage("抱歉，处理您的请求时出错了，请稍后重试。", nil), nil
		}

		return response, nil
	})
}

// ============================================
// Guard 条件检查（通过 compose.ProcessState 访问共享状态）
// ============================================

// checkPhase1Guard 检查 Phase 1 退出条件
// 通过 compose.ProcessState 读取共享状态，调用 phase.Phase1Guard 执行实际检查
func checkPhase1Guard(ctx context.Context, msg *schema.Message) (*agent.GuardResult, error) {
	var result *agent.GuardResult
	_ = compose.ProcessState(ctx, func(ctx context.Context, s *agent.ReimbursementState) error {
		result = phase.Phase1Guard(s)
		return nil
	})
	return result, nil
}

// checkPhase2Guard 检查 Phase 2 退出条件
// 通过 compose.ProcessState 读取共享状态，调用 phase.Phase2Guard 执行实际检查
func checkPhase2Guard(ctx context.Context, msg *schema.Message) (*agent.GuardResult, error) {
	var result *agent.GuardResult
	_ = compose.ProcessState(ctx, func(ctx context.Context, s *agent.ReimbursementState) error {
		result = phase.Phase2Guard(s)
		return nil
	})
	return result, nil
}

// ============================================
// 分支路由函数
// ============================================

// guard1Router Phase 1 守卫的分支路由
// passed=true → 进入 Phase 2；passed=false → 返回 Phase 1 继续收集信息
func guard1Router(ctx context.Context, result *agent.GuardResult) (string, error) {
	if result.Passed {
		return "phase2_validate", nil
	}
	return "phase1_collect", nil
}

// guard2Router Phase 2 守卫的分支路由
// passed=true → 进入 Phase 3；passed=false → 返回 Phase 2 继续校验确认
func guard2Router(ctx context.Context, result *agent.GuardResult) (string, error) {
	if result.Passed {
		return "phase3_execute", nil
	}
	return "phase2_validate", nil
}

// ============================================
// 辅助函数
// ============================================

// toolsToInfos 将 InvokableTool 列表转换为 ToolInfo 列表
// 供 model.WithTools 使用，因为 ChatModel.Generate 需要 []*schema.ToolInfo 参数
func toolsToInfos(ctx context.Context, tools []tool.InvokableTool) []*schema.ToolInfo {
	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	return infos
}
