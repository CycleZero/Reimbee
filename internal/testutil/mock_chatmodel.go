// Package testutil 提供测试基础设施，包括常用 Mock 实现
// 用于 Agent 层、Graph 编译层及其他内部模块的单元测试和集成测试
package testutil

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// MockChatModel 是一个可配置的 model.ToolCallingChatModel 模拟实现
// 用于测试中替代真实的 ChatModel（如 OpenAI/DeepSeek），
// 支持自定义 Generate、Stream、BindTools、WithTools 行为
//
// 使用方式：
//
//	mock := &testutil.MockChatModel{
//	    GenerateFunc: func(ctx, input, opts) (*schema.Message, error) {
//	        return schema.AssistantMessage("定制回复", nil), nil
//	    },
//	}
//
// 各字段为 nil 时使用默认行为（Generate 返回固定文本，Stream 单 chunk 返回）
type MockChatModel struct {
	// GenerateFunc 自定义 Generate 方法行为。为 nil 时返回默认 "mock reply" 消息
	GenerateFunc func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error)
	// StreamFunc 自定义 Stream 方法行为。为 nil 时返回单 chunk "mock reply" 流
	StreamFunc func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error)
	// BindToolsFunc 自定义 BindTools 方法行为。为 nil 时返回 nil（无错误）
	BindToolsFunc func(toolInfos []*schema.ToolInfo) error
	// WithToolsFunc 自定义 WithTools 方法行为。为 nil 时返回自身
	WithToolsFunc func(toolInfos []*schema.ToolInfo) (model.ToolCallingChatModel, error)
}

// 编译期验证 MockChatModel 实现了 model.ToolCallingChatModel 接口
var _ model.ToolCallingChatModel = (*MockChatModel)(nil)

// Generate 调用配置的 GenerateFunc，或返回默认消息 "mock reply"
func (m *MockChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m.GenerateFunc != nil {
		return m.GenerateFunc(ctx, input, opts...)
	}
	return schema.AssistantMessage("mock reply", nil), nil
}

// Stream 调用配置的 StreamFunc，或返回包含单条 "mock reply" 消息的流
func (m *MockChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if m.StreamFunc != nil {
		return m.StreamFunc(ctx, input, opts...)
	}
	// 默认：返回单 chunk 流
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		sw.Send(schema.AssistantMessage("mock reply", nil), nil)
		sw.Close()
	}()
	return sr, nil
}

// BindTools 调用配置的 BindToolsFunc，或返回 nil（无错误）
func (m *MockChatModel) BindTools(toolInfos []*schema.ToolInfo) error {
	if m.BindToolsFunc != nil {
		return m.BindToolsFunc(toolInfos)
	}
	return nil
}

// WithTools 调用配置的 WithToolsFunc，或返回自身
// 注意：返回自身符合 Mock 语义——测试中通常不关心工具绑定状态
func (m *MockChatModel) WithTools(toolInfos []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	if m.WithToolsFunc != nil {
		return m.WithToolsFunc(toolInfos)
	}
	return m, nil
}

// ============================================
// 预置场景辅助方法
// ============================================

// NewTextReplyChatModel 创建一个始终返回指定文本的 Mock ChatModel
// 适用于测试 ChatModel 直出场景（无工具调用）
func NewTextReplyChatModel(reply string) *MockChatModel {
	return &MockChatModel{
		GenerateFunc: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
			return schema.AssistantMessage(reply, nil), nil
		},
		StreamFunc: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			sr, sw := schema.Pipe[*schema.Message](1)
			go func() {
				sw.Send(schema.AssistantMessage(reply, nil), nil)
				sw.Close()
			}()
			return sr, nil
		},
	}
}

// NewToolCallChatModel 创建一个返回 ToolCall 消息的 Mock ChatModel
// 适用于测试 ReAct 循环的工具调用分支
// toolCalls: 要返回的 ToolCall 列表
func NewToolCallChatModel(toolCalls []schema.ToolCall) *MockChatModel {
	return &MockChatModel{
		GenerateFunc: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
			return &schema.Message{
				Role:      schema.Assistant,
				Content:   "",
				ToolCalls: toolCalls,
			}, nil
		},
	}
}

// NewMultiTurnChatModel 创建一个支持多轮交互的 Mock ChatModel
// 按顺序返回 responses 中的消息。最后一次返回后，继续返回最后一次的消息
// 适用于测试 "LLM → Tool → LLM → ..." 多轮对话场景
// responses: 按调用顺序返回的消息列表
func NewMultiTurnChatModel(responses []*schema.Message) *MockChatModel {
	callCount := 0
	return &MockChatModel{
		GenerateFunc: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
			idx := callCount
			callCount++
			if idx >= len(responses) {
				// 超出预设列表后返回最后一条
				return responses[len(responses)-1], nil
			}
			return responses[idx], nil
		},
	}
}
