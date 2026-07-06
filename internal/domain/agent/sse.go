package agent

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ============================================
// SSE 事件类型定义（对应 agent-design-supplement.md §4 的 8 种类型）
// ============================================

// SSEEventType SSE 事件类型枚举
type SSEEventType string

const (
	EventTypeThinking        SSEEventType = "thinking"         // LLM 思考中（三点动画 + 文字提示）
	EventTypeToolCall         SSEEventType = "tool_call"        // 工具调用开始
	EventTypeToolResult       SSEEventType = "tool_result"      // 工具调用完成
	EventTypeMessage          SSEEventType = "message"          // LLM 文本输出（delta=true 为流式增量）
	EventTypePhaseChange      SSEEventType = "phase_change"     // 阶段切换
	EventTypeConfirmRequired  SSEEventType = "confirm_required" // 需要用户确认操作
	EventTypeError            SSEEventType = "error"            // 错误事件
	EventTypeDone             SSEEventType = "done"             // 流程完成
)

// ============================================
// SSE 事件结构
// ============================================

// SSEEvent 通用 SSE 事件结构
// Type 决定 Data 的解析方式，前端根据 Type 渲染不同的 UI 组件
type SSEEvent struct {
	Type SSEEventType `json:"type"` // 事件类型
	Data any          `json:"data"` // 事件负载（随 Type 变化）
}

// ThinkingData thinking 事件的负载
type ThinkingData struct {
	Message string `json:"message"` // 思考状态的文字描述（如"正在识别票据..."）
}

// ToolCallData tool_call 事件的负载
type ToolCallData struct {
	Tool  string `json:"tool"`  // 工具名称（如 recognize_invoice）
	Input any    `json:"input"` // 工具输入参数
}

// ToolResultData tool_result 事件的负载
type ToolResultData struct {
	Tool   string `json:"tool"`   // 工具名称
	Output any    `json:"output"` // 工具输出结果
}

// MessageData message 事件的负载
type MessageData struct {
	Content string `json:"content"` // 文本内容
	Delta   bool   `json:"delta"`   // 是否为增量流式输出（true 时前端追加，false 时替换）
}

// PhaseChangeData phase_change 事件的负载
type PhaseChangeData struct {
	From    string `json:"from"`    // 来源阶段
	To      string `json:"to"`      // 目标阶段
	Summary string `json:"summary"` // 阶段过渡摘要
}

// ConfirmRequiredData confirm_required 事件的负载
type ConfirmRequiredData struct {
	Prompt  string `json:"prompt"`  // 确认提示文字
	Action  string `json:"action"`  // 确认动作标识（如 confirm_invoice / confirm_submit）
	Context any    `json:"context"` // 确认所需的上下文数据
}

// ErrorData error 事件的负载
type ErrorData struct {
	Message string `json:"message"` // 错误描述
	Retry   bool   `json:"retry"`   // 是否可重试
	Code    string `json:"code"`    // 错误代码（用于前端分类处理）
}

// ============================================
// SSE 事件工厂函数
// ============================================

// NewThinkingEvent 创建 thinking 事件
func NewThinkingEvent(message string) SSEEvent {
	return SSEEvent{
		Type: EventTypeThinking,
		Data: ThinkingData{Message: message},
	}
}

// NewToolCallEvent 创建 tool_call 事件
func NewToolCallEvent(toolName string, input any) SSEEvent {
	return SSEEvent{
		Type: EventTypeToolCall,
		Data: ToolCallData{Tool: toolName, Input: input},
	}
}

// NewToolResultEvent 创建 tool_result 事件
func NewToolResultEvent(toolName string, output any) SSEEvent {
	return SSEEvent{
		Type: EventTypeToolResult,
		Data: ToolResultData{Tool: toolName, Output: output},
	}
}

// NewMessageEvent 创建 message 事件
func NewMessageEvent(content string, delta bool) SSEEvent {
	return SSEEvent{
		Type: EventTypeMessage,
		Data: MessageData{Content: content, Delta: delta},
	}
}

// NewPhaseChangeEvent 创建 phase_change 事件
func NewPhaseChangeEvent(from, to, summary string) SSEEvent {
	return SSEEvent{
		Type: EventTypePhaseChange,
		Data: PhaseChangeData{From: from, To: to, Summary: summary},
	}
}

// NewConfirmRequiredEvent 创建 confirm_required 事件
func NewConfirmRequiredEvent(prompt, action string, context any) SSEEvent {
	return SSEEvent{
		Type: EventTypeConfirmRequired,
		Data: ConfirmRequiredData{Prompt: prompt, Action: action, Context: context},
	}
}

// NewErrorEvent 创建 error 事件
func NewErrorEvent(message string, retry bool, code string) SSEEvent {
	return SSEEvent{
		Type: EventTypeError,
		Data: ErrorData{Message: message, Retry: retry, Code: code},
	}
}

// NewDoneEvent 创建 done 事件
func NewDoneEvent() SSEEvent {
	return SSEEvent{
		Type: EventTypeDone,
		Data: nil,
	}
}

// ============================================
// SSE Writer（Gin 实现）
// ============================================

// SSEWriter SSE 事件写入接口
type SSEWriter interface {
	WriteEvent(event SSEEvent) error
	Flush() error
}

// GinSSEWriter 基于 Gin 的 SSE 事件写入器
// 实现 SSEWriter 接口，通过 gin.Context 和 http.Flusher 支持流式输出
type GinSSEWriter struct {
	c       *gin.Context
	flusher http.Flusher
}

// NewGinSSEWriter 创建 Gin SSE 写入器
// 调用前需确保已设置 SSE 响应头（Content-Type: text/event-stream 等）
func NewGinSSEWriter(c *gin.Context) (*GinSSEWriter, error) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("当前 HTTP 响应不支持流式写入")
	}

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁用 Nginx 缓冲

	// 立即发送响应头
	c.Writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	return &GinSSEWriter{c: c, flusher: flusher}, nil
}

// WriteEvent 写入一个 SSE 事件到响应流
// 格式: event: <type>\ndata: <json>\n\n
func (w *GinSSEWriter) WriteEvent(event SSEEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("序列化SSE事件失败: %w", err)
	}

	// SSE 标准格式：event: <type> + data: <json> + 空行分隔
	output := fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, string(data))

	if _, err := w.c.Writer.WriteString(output); err != nil {
		return fmt.Errorf("写入SSE事件失败: %w", err)
	}

	return nil
}

// Flush 刷新缓冲区，将数据推送到客户端
func (w *GinSSEWriter) Flush() error {
	w.flusher.Flush()
	return nil
}

// Close 关闭 SSE 连接（Gin 自动管理连接生命周期，此方法为空操作）
func (w *GinSSEWriter) Close() error {
	return nil
}
