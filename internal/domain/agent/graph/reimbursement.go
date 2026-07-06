// Package graph Graph 定义层 —— 构建 Eino compose.Graph 编译为 Runnable
//
// 本文件定义报销三阶段主流程的顶层 Graph，
// 每个阶段通过 buildReActPhase() 构建为标准 ReAct 子图，
// 阶段间通过 Guard Lambda + AddBranch 控制流转条件
package graph

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/internal/domain/agent/phase"
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ============================================
// 依赖结构体
// ============================================

// ReimbursementGraphDeps 报销流程图构建所需的运行时依赖
// ChatModel 和 ToolSet 在测试场景下可为 nil（构建阶段报错）
type ReimbursementGraphDeps struct {
	Logger    *log.Logger               // 结构化日志记录器
	ToolSet   *tools.ToolSet            // 工具集（按三阶段分组提供 BaseTool）
	ChatModel model.ToolCallingChatModel // LLM 聊天模型（需支持 Tool Calling）
	Config    *agent.AgentConfig        // 智能体运行时配置
}

// ============================================
// NewReimbursementGraph — 主构建函数
// ============================================

// NewReimbursementGraph 构建报销三阶段主流程图
//
// 图类型: []*schema.Message → *schema.Message（与 ReAct 阶段子图一致）
//
// 顶层拓扑:
//
//	START → Phase1(ReAct子图) → phase1_guard ──[pass]──→ Phase2(ReAct子图) → phase2_guard ──[pass]──→ Phase3(ReAct子图) → END
//	                               │   [fail]                                        │   [fail]
//	                               └──→ Phase1 (重试)                                 └──→ Phase2 (重试)
//
// 每个 Phase 内部是独立的 ReAct 循环（ChatModel ↔ ToolsNode），
// Guard 节点通过 compose.ProcessState 检查 ReimbursementState 决定是否进入下一阶段。
//
// 防死循环: compose.WithMaxRunSteps(100) 全局限制图引擎最大执行步数
func NewReimbursementGraph(
	ctx context.Context,
	deps ReimbursementGraphDeps,
) (compose.Runnable[[]*schema.Message, *schema.Message], error) {
	deps.Logger.Debug("开始构建报销子流程 Graph")

	buildCtx := context.Background() // 构建阶段使用独立上下文

	// ── 第一阶段：构建三个 ReAct 阶段子图 ──
	// 每个子图内部为标准的 ChatModel → ToolsNode → Branch 循环，
	// 工具通过各阶段的 Tools 列表独立绑定到 ChatModel

	// Phase 1: 信息收集 —— OCR 票据识别 + 合规标准查询
	phase1Graph, err := buildReActPhase(buildCtx, deps.ChatModel, deps.Logger, PhaseConfig{
		Name:         "phase1_collect",
		SystemPrompt: agent.BuildSystemPrompt("phase1_collect", nil),
		Tools:        getPhaseBaseTools(deps.ToolSet, 1),
	})
	if err != nil {
		return nil, fmt.Errorf("构建Phase1子图失败: %w", err)
	}

	// Phase 2: 校验确认 —— 合规检查 + 预算检查
	phase2Graph, err := buildReActPhase(buildCtx, deps.ChatModel, deps.Logger, PhaseConfig{
		Name:         "phase2_validate",
		SystemPrompt: agent.BuildSystemPrompt("phase2_validate", nil),
		Tools:        getPhaseBaseTools(deps.ToolSet, 2),
	})
	if err != nil {
		return nil, fmt.Errorf("构建Phase2子图失败: %w", err)
	}

	// Phase 3: 执行提交 —— PDF 生成 + 邮件发送 + 进度告知
	phase3Graph, err := buildReActPhase(buildCtx, deps.ChatModel, deps.Logger, PhaseConfig{
		Name:         "phase3_execute",
		SystemPrompt: agent.BuildSystemPrompt("phase3_execute", nil),
		Tools:        getPhaseBaseTools(deps.ToolSet, 3),
	})
	if err != nil {
		return nil, fmt.Errorf("构建Phase3子图失败: %w", err)
	}

	deps.Logger.Debug("三阶段ReAct子图构建完成，开始组装父图")

	// ── 第二阶段：创建父图（注入 ReimbursementState 共享状态）──
	// 父图与 Guard/Branch 节点都通过 compose.ProcessState 访问此状态
	g := compose.NewGraph[[]*schema.Message, *schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *agent.ReimbursementState {
			return &agent.ReimbursementState{CurrentPhase: "phase1_collect"}
		}),
	)

	// ── 第三阶段：将三个 ReAct 子图挂载为父图节点 ──
	// StatePreHandler: 每次进入阶段时更新 CurrentPhase 标记

	err = g.AddGraphNode("phase1_collect", phase1Graph,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, rs *agent.ReimbursementState) ([]*schema.Message, error) {
			rs.CurrentPhase = "phase1_collect"
			return input, nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("添加Phase1子图节点失败: %w", err)
	}

	err = g.AddGraphNode("phase2_validate", phase2Graph,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, rs *agent.ReimbursementState) ([]*schema.Message, error) {
			rs.CurrentPhase = "phase2_validate"
			return input, nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("添加Phase2子图节点失败: %w", err)
	}

	err = g.AddGraphNode("phase3_execute", phase3Graph,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, rs *agent.ReimbursementState) ([]*schema.Message, error) {
			rs.CurrentPhase = "phase3_execute"
			return input, nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("添加Phase3子图节点失败: %w", err)
	}

	// ── 第四阶段：添加 Guard Lambda 节点 ──
	// Guard Lambda 职责:
	//   1. 接收子图 END 输出的单条 *schema.Message
	//   2. 封装为 []*schema.Message（匹配下游子图 START 输入类型）
	//   3. 递增对应阶段的轮次计数器（Phase1Turns / Phase2Turns）
	//   4. 调用 phase.PhaseXGuard() 记录护卫检查日志
	// 实际路由决策由下游 AddBranch 中的条件函数负责

	// Phase 1 Guard：检查票据收集是否完成，决定是进入 Phase2 还是重试 Phase1
	g.AddLambdaNode("phase1_guard", compose.InvokableLambda(
		func(ctx context.Context, msg *schema.Message) ([]*schema.Message, error) {
			_ = compose.ProcessState(ctx, func(ctx context.Context, rs *agent.ReimbursementState) error {
				rs.Phase1Turns++
				result := phase.Phase1Guard(rs)
				deps.Logger.Debug("Phase1 护卫检查",
					zap.Bool("通过", result.Passed),
					zap.Int("累计轮次", rs.Phase1Turns),
					zap.String("原因", result.Reason),
				)
				return nil
			})
			return []*schema.Message{msg}, nil
		},
	))

	// Phase 2 Guard：检查合规校验与用户确认是否完成
	g.AddLambdaNode("phase2_guard", compose.InvokableLambda(
		func(ctx context.Context, msg *schema.Message) ([]*schema.Message, error) {
			_ = compose.ProcessState(ctx, func(ctx context.Context, rs *agent.ReimbursementState) error {
				rs.Phase2Turns++
				result := phase.Phase2Guard(rs)
				deps.Logger.Debug("Phase2 护卫检查",
					zap.Bool("通过", result.Passed),
					zap.Int("累计轮次", rs.Phase2Turns),
					zap.String("原因", result.Reason),
				)
				return nil
			})
			return []*schema.Message{msg}, nil
		},
	))

	// ── 第五阶段：添加分支逻辑 ──
	// 分支条件函数接收 Guard Lambda 的输出（[]*schema.Message），
	// 通过 compose.ProcessState 读取 ReimbursementState 重新调用 Guard 判断是否放行

	// Phase1Guard 分支：通过 → Phase2，失败 → 回到 Phase1 重试
	err = g.AddBranch("phase1_guard", compose.NewGraphBranch(
		func(ctx context.Context, msgs []*schema.Message) (string, error) {
			var nextNode string
			_ = compose.ProcessState(ctx, func(ctx context.Context, rs *agent.ReimbursementState) error {
				result := phase.Phase1Guard(rs)
				if result.Passed {
					nextNode = "phase2_validate"
					deps.Logger.Info("Phase1 护卫通过，进入Phase2校验阶段",
						zap.Int("票据数", len(rs.Invoices)),
						zap.Int("总轮次", rs.Phase1Turns),
					)
				} else {
					nextNode = "phase1_collect"
					deps.Logger.Debug("Phase1 护卫未通过，继续收集信息",
						zap.String("原因", result.Reason),
					)
				}
				return nil
			})
			return nextNode, nil
		},
		map[string]bool{
			"phase1_collect":  true, // fail → 循环回 Phase1
			"phase2_validate": true, // pass → 进入 Phase2
		},
	))
	if err != nil {
		return nil, fmt.Errorf("添加Phase1 Guard分支失败: %w", err)
	}

	// Phase2Guard 分支：通过 → Phase3，失败 → 回到 Phase2 重试
	err = g.AddBranch("phase2_guard", compose.NewGraphBranch(
		func(ctx context.Context, msgs []*schema.Message) (string, error) {
			var nextNode string
			_ = compose.ProcessState(ctx, func(ctx context.Context, rs *agent.ReimbursementState) error {
				result := phase.Phase2Guard(rs)
				if result.Passed {
					nextNode = "phase3_execute"
					deps.Logger.Info("Phase2 护卫通过，进入Phase3执行提交阶段",
						zap.Int("总轮次", rs.Phase2Turns),
					)
				} else {
					nextNode = "phase2_validate"
					deps.Logger.Debug("Phase2 护卫未通过，继续校验确认",
						zap.String("原因", result.Reason),
					)
				}
				return nil
			})
			return nextNode, nil
		},
		map[string]bool{
			"phase2_validate": true, // fail → 循环回 Phase2
			"phase3_execute":  true, // pass → 进入 Phase3
		},
	))
	if err != nil {
		return nil, fmt.Errorf("添加Phase2 Guard分支失败: %w", err)
	}

	// ── 第六阶段：连线 —— 定义节点间的有向数据流 ──

	// START → Phase1（用户输入消息数组进入第一阶段）
	_ = g.AddEdge(compose.START, "phase1_collect")

	// Phase1 → Phase1Guard（Phase1 的输出经 Guard 检查后由分支决定去向）
	_ = g.AddEdge("phase1_collect", "phase1_guard")

	// Phase2 → Phase2Guard（Phase2 的输出经 Guard 检查后由分支决定去向）
	_ = g.AddEdge("phase2_validate", "phase2_guard")

	// Phase3 → END（Phase3 完成后流程结束，无需 Guard）
	_ = g.AddEdge("phase3_execute", compose.END)

	// ── 第七阶段：编译图 ──
	runnable, err := g.Compile(ctx,
		compose.WithGraphName("reimbursement_workflow"),
		compose.WithMaxRunSteps(100),
	)
	if err != nil {
		deps.Logger.Error("编译报销子流程Graph失败", zap.Error(err))
		return nil, fmt.Errorf("编译报销子流程Graph失败: %w", err)
	}

	deps.Logger.Info("报销子流程 Graph 编译成功",
		zap.String("图名", "reimbursement_workflow"),
		zap.Int("最大步数", 100),
	)
	return runnable, nil
}

// ============================================
// 工具函数
// ============================================

// getPhaseBaseTools 安全获取指定阶段的工具列表
// ToolSet 为 nil 时返回空列表（测试场景兼容）
func getPhaseBaseTools(ts *tools.ToolSet, phase int) []tool.BaseTool {
	if ts == nil {
		return nil
	}
	switch phase {
	case 1:
		return ts.GetPhase1BaseTools()
	case 2:
		return ts.GetPhase2BaseTools()
	case 3:
		return ts.GetPhase3BaseTools()
	default:
		return nil
	}
}
