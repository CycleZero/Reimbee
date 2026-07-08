// Package tools 中断流程演示工具
//
// 使用 Interruptable 包装——无需手动处理两阶段逻辑。
package tools

import (
	"context"

	"github.com/CycleZero/blades/tools"
)

// 审批状态 context key
type approvalCtxKey struct{}

// ApprovalState 审批状态（单次有效，消费后需重新确认）
type ApprovalState struct {
	Approved bool
	Reason   string
	Consumed bool
}

// TestInterruptTool 中断流程演示工具
type TestInterruptTool struct{ tools.Tool }

// NewTestInterruptTool 创建中断测试工具（Interruptable 包装）
func NewTestInterruptTool() *TestInterruptTool {
	base, err := tools.NewFunc[TestInterruptInput, TestInterruptOutput](
		"test_interrupt",
		"中断流程演示工具。调用后触发审批流程等待用户确认。",
		func(ctx context.Context, _ TestInterruptInput) (TestInterruptOutput, error) {
			return TestInterruptOutput{Phase: 2, Message: "中断恢复成功，操作已执行"}, nil
		},
	)
	if err != nil {
		panic("创建interrupt测试工具失败: " + err.Error())
	}
	return &TestInterruptTool{NewInterruptable(base, "请确认执行中断测试操作？")}
}

type TestInterruptInput  struct{}
type TestInterruptOutput struct{ Phase int; Message string }

// ── 审批状态 context 注入/读取 ──

func SetApprovalState(ctx context.Context, state *ApprovalState) context.Context {
	return context.WithValue(ctx, approvalCtxKey{}, state)
}

func InjectApprovalState(ctx context.Context, s map[string]any) context.Context {
	if raw, ok := s["approval"]; ok {
		switch v := raw.(type) {
		case *ApprovalState:
			return SetApprovalState(ctx, v)
		case ApprovalState:
			return SetApprovalState(ctx, &v)
		}
	}
	return ctx
}

func getApprovalState(ctx context.Context) *ApprovalState {
	if v, ok := ctx.Value(approvalCtxKey{}).(*ApprovalState); ok {
		return v
	}
	return nil
}
