// Package testutil 提供测试基础设施，包括常用 Mock 实现
package testutil

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// MockBaseTool 是一个可配置的 tool.BaseTool + tool.InvokableTool 模拟实现
// 用于测试中替代真实工具（如 OCR、Compliance、Budget 等），
// 支持自定义 Info 和 InvokableRun 行为
//
// 使用方式：
//
//	mock := &testutil.MockBaseTool{
//	    InfoFunc: func(ctx) (*schema.ToolInfo, error) {
//	        return &schema.ToolInfo{Name: "my_tool", Desc: "测试工具"}, nil
//	    },
//	    RunFunc: func(ctx, argsJSON) (string, error) {
//	        return "工具执行结果", nil
//	    },
//	}
type MockBaseTool struct {
	// InfoFunc 自定义 Info 方法行为。为 nil 时返回默认 ToolInfo（名称 "mock_tool"）
	InfoFunc func(ctx context.Context) (*schema.ToolInfo, error)
	// RunFunc 自定义 InvokableRun 方法行为。为 nil 时返回 "mock tool result"
	RunFunc func(ctx context.Context, argumentsInJSON string) (string, error)
}

// 编译期验证 MockBaseTool 实现了 tool.BaseTool + tool.InvokableTool 接口
var _ tool.BaseTool = (*MockBaseTool)(nil)
var _ tool.InvokableTool = (*MockBaseTool)(nil)

// Info 调用配置的 InfoFunc，或返回默认 ToolInfo
func (m *MockBaseTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	if m.InfoFunc != nil {
		return m.InfoFunc(ctx)
	}
	return &schema.ToolInfo{
		Name: "mock_tool",
		Desc: "一个模拟工具，用于测试",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"input": {
				Type:     schema.String,
				Desc:     "输入参数",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun 调用配置的 RunFunc，或返回 "mock tool result"
func (m *MockBaseTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	if m.RunFunc != nil {
		return m.RunFunc(ctx, argumentsInJSON)
	}
	return "mock tool result", nil
}

// ============================================
// 预置场景辅助方法
// ============================================

// NewNamedMockTool 创建一个带有指定名称的 Mock 工具
// name: 工具名称（用于 ToolCall.Function.Name 匹配）
// result: InvokableRun 返回的固定结果
func NewNamedMockTool(name string, result string) *MockBaseTool {
	return &MockBaseTool{
		InfoFunc: func(ctx context.Context) (*schema.ToolInfo, error) {
			return &schema.ToolInfo{
				Name: name,
				Desc: "测试工具: " + name,
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"input": {
						Type:     schema.String,
						Desc:     "输入参数",
						Required: true,
					},
				}),
			}, nil
		},
		RunFunc: func(ctx context.Context, argumentsInJSON string) (string, error) {
			return result, nil
		},
	}
}
