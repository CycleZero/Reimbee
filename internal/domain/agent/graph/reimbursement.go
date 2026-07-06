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

type ReimbursementGraphDeps struct {
	Logger    *log.Logger
	ToolSet   *tools.ToolSet
	ChatModel einoModel.ToolCallingChatModel
	Config    *agent.AgentConfig
}

// NewReimbursementGraph 构建报销三阶段子流程 Graph
// Graph 类型：[*schema.Message, *schema.Message] — 消息进，消息出
// 采用线性拓扑（无分支）— 每个阶段内部自行循环直到 Guards 通过
// START → Phase1 → Phase2 → Phase3 → END
func NewReimbursementGraph(ctx context.Context, deps ReimbursementGraphDeps) (compose.Runnable[*schema.Message, *schema.Message], error) {
	deps.Logger.Debug("开始构建报销子流程 Graph")

	g := compose.NewGraph[*schema.Message, *schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *agent.ReimbursementState {
			return &agent.ReimbursementState{CurrentPhase: "phase1_collect"}
		}),
	)

	p1Tools := toolInfosForPhase(ctx, deps.ToolSet, 1)
	p2Tools := toolInfosForPhase(ctx, deps.ToolSet, 2)
	p3Tools := toolInfosForPhase(ctx, deps.ToolSet, 3)

	p1Prompt := agent.BuildSystemPrompt("phase1_collect", nil)
	p2Prompt := agent.BuildSystemPrompt("phase2_validate", nil)
	p3Prompt := agent.BuildSystemPrompt("phase3_execute", nil)

	g.AddLambdaNode("phase1_collect", newPhaseWithGuard(
		deps.ChatModel, p1Prompt, p1Tools, deps.Logger, "phase1",
		func(s *agent.ReimbursementState) *agent.GuardResult { return phase.Phase1Guard(s) },
		maxPhaseTurns(deps),
	))
	g.AddLambdaNode("phase2_validate", newPhaseWithGuard(
		deps.ChatModel, p2Prompt, p2Tools, deps.Logger, "phase2",
		func(s *agent.ReimbursementState) *agent.GuardResult { return phase.Phase2Guard(s) },
		maxPhaseTurns(deps),
	))
	g.AddLambdaNode("phase3_execute", newPhaseNodeSimple(
		deps.ChatModel, p3Prompt, p3Tools, deps.Logger, "phase3",
	))

	g.AddEdge(compose.START, "phase1_collect")
	g.AddEdge("phase1_collect", "phase2_validate")
	g.AddEdge("phase2_validate", "phase3_execute")
	g.AddEdge("phase3_execute", compose.END)

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

// newPhaseWithGuard 创建带 Guard 循环的阶段节点
// 每次进入阶段：调用 LLM → 检查 Guard → 通过则返回消息 → 未通过则循环重试（利用 ProcessState 更新状态）
func newPhaseWithGuard(
	chatModel einoModel.ToolCallingChatModel,
	systemPrompt string,
	toolInfos []*schema.ToolInfo,
	logger *log.Logger,
	phaseName string,
	guardFn func(*agent.ReimbursementState) *agent.GuardResult,
	maxTurns int,
) *compose.Lambda {
	return compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) (*schema.Message, error) {
		for turn := 0; turn < maxTurns; turn++ {
			// 调用 LLM
			messages := []*schema.Message{
				schema.SystemMessage(systemPrompt),
				msg,
			}
			response, err := chatModel.Generate(ctx, messages, einoModel.WithTools(toolInfos))
			if err != nil {
				logger.Error("LLM调用失败", zap.String("阶段", phaseName), zap.Error(err))
				return schema.AssistantMessage("抱歉，处理您的请求时出错了，请稍后重试。", nil), nil
			}

			// 更新用户消息（下一轮用 LLM 的回复作为上下文）
			msg = schema.UserMessage(response.Content)

			// 检查 Guard
			var passed bool
			var guardMsg string
			_ = compose.ProcessState(ctx, func(ctx context.Context, s *agent.ReimbursementState) error {
				s.Phase1Turns++
				result := guardFn(s)
				passed = result.Passed
				guardMsg = result.Message
				return nil
			})

			if passed {
				return response, nil
			}
			// Guard 未通过——将提示注入到下一轮的用户消息中
			msg = schema.UserMessage(guardMsg)
		}
		return schema.AssistantMessage("操作步骤过多，请稍后重试。", nil), nil
	})
}

// newPhaseNodeSimple 无 Guard 的阶段节点（Phase 3）
func newPhaseNodeSimple(
	chatModel einoModel.ToolCallingChatModel,
	systemPrompt string,
	toolInfos []*schema.ToolInfo,
	logger *log.Logger,
	phaseName string,
) *compose.Lambda {
	return compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) (*schema.Message, error) {
		messages := []*schema.Message{
			schema.SystemMessage(systemPrompt),
			msg,
		}
		response, err := chatModel.Generate(ctx, messages, einoModel.WithTools(toolInfos))
		if err != nil {
			logger.Error("LLM调用失败", zap.String("阶段", phaseName), zap.Error(err))
			return schema.AssistantMessage("抱歉，处理您的请求时出错了，请稍后重试。", nil), nil
		}
		return response, nil
	})
}

func toolInfosForPhase(ctx context.Context, ts *tools.ToolSet, phase int) []*schema.ToolInfo {
	if ts == nil {
		return nil
	}
	var toolList []tool.InvokableTool
	switch phase {
	case 1:
		toolList = ts.GetPhase1Tools()
	case 2:
		toolList = ts.GetPhase2Tools()
	case 3:
		toolList = ts.GetPhase3Tools()
	}
	infos := make([]*schema.ToolInfo, 0, len(toolList))
	for _, t := range toolList {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	return infos
}

func maxPhaseTurns(deps ReimbursementGraphDeps) int {
	if deps.Config != nil && deps.Config.MaxPhaseTurns > 0 {
		return deps.Config.MaxPhaseTurns
	}
	return 10
}
