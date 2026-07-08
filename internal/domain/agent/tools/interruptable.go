// Package tools 可中断工具抽象
//
// InterruptableTool 封装两阶段中断模式：
//   - 首次调用：触发中断等待用户确认
//   - 确认后恢复：执行真实工具逻辑
//   - 确认单次有效：消费后下次调用重新中断
//
// 使用方式：
//
//	baseTool, _ := tools.NewFunc[I, O]("name", "desc", handler)
//	interruptTool := NewInterruptable(baseTool, "确认执行操作？")
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/CycleZero/blades/tools"
	"github.com/google/jsonschema-go/jsonschema"
)

// Interruptable 包裹一个 Tool，使其具有中断确认能力。
// inner 的 Handle 仅在用户确认后被调用。
type Interruptable struct {
	inner      tools.Tool
	confirmMsg string // 确认提示文案
}

// NewInterruptable 创建可中断工具
func NewInterruptable(inner tools.Tool, confirmMsg string) *Interruptable {
	return &Interruptable{inner: inner, confirmMsg: confirmMsg}
}

// ── tools.Tool 接口 ──

func (t *Interruptable) Name() string               { return t.inner.Name() }
func (t *Interruptable) Description() string         { return t.inner.Description() }
func (t *Interruptable) InputSchema() *jsonschema.Schema  { return t.inner.InputSchema() }
func (t *Interruptable) OutputSchema() *jsonschema.Schema { return t.inner.OutputSchema() }

// Handle 两阶段执行
func (t *Interruptable) Handle(ctx context.Context, input string) (string, error) {
	tc, _ := tools.FromContext(ctx)
	state := getApprovalState(ctx)

	if state == nil || state.Consumed {
		// 第一阶段：未批准或已消费 → 中断
		if tc != nil {
			tc.SetAction("await_approval", t.confirmMsg)
			tc.SetAction(tools.ActionLoopExit, true)
		}
		return mustJSON(map[string]any{
			"status":  "pending",
			"message": "等待用户确认",
		}), nil
	}

	if state.Approved && !state.Consumed {
		// 第二阶段：已批准 → 执行
		state.Consumed = true
		return t.inner.Handle(ctx, input)
	}

	// 被拒绝
	return mustJSON(map[string]any{
		"status":  "rejected",
		"message": fmt.Sprintf("用户拒绝: %s", state.Reason),
	}), nil
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
