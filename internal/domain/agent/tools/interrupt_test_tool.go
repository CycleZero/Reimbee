// Package tools 中断流程演示工具
//
// 演示完整的 interrupt 两阶段流程：
//   第一阶段（未确认）：设 ActionLoopExit → 循环退出 → 前端弹确认框
//   第二阶段（已确认）：正常返回结果
//
// 确认状态通过 ToolContext 读取：首次调用无状态 → 中断；恢复后 context 带标记 → 执行。
package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/blades/tools"
)

// TestInterruptInput 中断测试工具输入
type TestInterruptInput struct {
	Confirm bool `json:"confirm" jsonschema_description:"是否已确认（第一阶段传false，第二阶段传true）"`
}

// TestInterruptOutput 中断测试工具输出
type TestInterruptOutput struct {
	Phase   int    `json:"phase"`            // 阶段：1=已中断 2=已执行
	Message string `json:"message"`          // 提示信息
	Payload string `json:"payload,omitempty"` // 第二阶段返回的业务数据
}

// TestInterruptTool 中断流程演示工具
type TestInterruptTool struct{ tools.Tool }

// NewTestInterruptTool 创建中断测试工具
func NewTestInterruptTool() *TestInterruptTool {
	t, err := tools.NewFunc[TestInterruptInput, TestInterruptOutput](
		"test_interrupt",
		"中断流程演示工具。首次调用（confirm=false）触发中断等待用户确认，确认后（confirm=true）返回演示数据。",
		func(ctx context.Context, input TestInterruptInput) (TestInterruptOutput, error) {
			tc, ok := tools.FromContext(ctx)

			// 第一阶段：未确认 → 触发中断
			if !input.Confirm {
				fmt.Println("[InterruptTest] 第一阶段：触发中断，等待用户确认")
				if ok {
					tc.SetAction("await_approval",
						"请确认执行中断测试操作？（演示用）金额：¥123.45")
					tc.SetAction(tools.ActionLoopExit, true)
				}
				return TestInterruptOutput{
					Phase:   1,
					Message: "中断已触发，等待用户确认",
				}, nil
			}

			// 第二阶段：已确认 → 执行实际操作
			fmt.Println("[InterruptTest] 第二阶段：用户已确认，执行操作")
			return TestInterruptOutput{
				Phase:   2,
				Message: "中断恢复成功，操作已执行",
				Payload: `{"status":"done","timestamp":"2026-07-08T10:00:00Z","confirmed_by":"user"}`,
			}, nil
		},
	)
	if err != nil {
		panic("创建interrupt测试工具失败: " + err.Error())
	}
	return &TestInterruptTool{t}
}
