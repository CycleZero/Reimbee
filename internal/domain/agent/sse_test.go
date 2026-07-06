package agent_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ============================================
// SSE 事件工厂函数测试
// ============================================

// TestNewThinkingEvent 验证 thinking 事件的结构正确
func TestNewThinkingEvent(t *testing.T) {
	event := agent.NewThinkingEvent("正在识别票据信息...")

	if event.Type != agent.EventTypeThinking {
		t.Errorf("事件类型应为 thinking，实际为: %s", event.Type)
	}

	data, ok := event.Data.(agent.ThinkingData)
	if !ok {
		t.Fatal("Data 应为 ThinkingData 类型")
	}
	if data.Message != "正在识别票据信息..." {
		t.Errorf("消息内容应为'正在识别票据信息...'，实际为: %s", data.Message)
	}
}

// TestNewToolCallEvent 验证 tool_call 事件的结构正确
func TestNewToolCallEvent(t *testing.T) {
	input := map[string]string{"image_path": "/tmp/invoice.png"}
	event := agent.NewToolCallEvent("recognize_invoice", input)

	if event.Type != agent.EventTypeToolCall {
		t.Errorf("事件类型应为 tool_call，实际为: %s", event.Type)
	}

	data, ok := event.Data.(agent.ToolCallData)
	if !ok {
		t.Fatal("Data 应为 ToolCallData 类型")
	}
	if data.Tool != "recognize_invoice" {
		t.Errorf("工具名应为'recognize_invoice'，实际为: %s", data.Tool)
	}
}

// TestNewToolResultEvent 验证 tool_result 事件的结构正确
func TestNewToolResultEvent(t *testing.T) {
	output := map[string]any{"amount": 100.50, "category": "差旅-交通"}
	event := agent.NewToolResultEvent("recognize_invoice", output)

	if event.Type != agent.EventTypeToolResult {
		t.Errorf("事件类型应为 tool_result，实际为: %s", event.Type)
	}

	data, ok := event.Data.(agent.ToolResultData)
	if !ok {
		t.Fatal("Data 应为 ToolResultData 类型")
	}
	if data.Tool != "recognize_invoice" {
		t.Errorf("工具名应为'recognize_invoice'，实际为: %s", data.Tool)
	}
}

// TestNewMessageEvent_Delta 验证消息事件 delta=true 时结构正确
func TestNewMessageEvent_Delta(t *testing.T) {
	event := agent.NewMessageEvent("你好", true)

	if event.Type != agent.EventTypeMessage {
		t.Errorf("事件类型应为 message，实际为: %s", event.Type)
	}

	data, ok := event.Data.(agent.MessageData)
	if !ok {
		t.Fatal("Data 应为 MessageData 类型")
	}
	if data.Content != "你好" {
		t.Errorf("内容应为'你好'，实际为: %s", data.Content)
	}
	if !data.Delta {
		t.Error("delta 应为 true")
	}
}

// TestNewMessageEvent_Full 验证消息事件 delta=false（完整消息）时结构正确
func TestNewMessageEvent_Full(t *testing.T) {
	event := agent.NewMessageEvent("报销单已提交", false)

	if event.Type != agent.EventTypeMessage {
		t.Errorf("事件类型应为 message，实际为: %s", event.Type)
	}

	data, ok := event.Data.(agent.MessageData)
	if !ok {
		t.Fatal("Data 应为 MessageData 类型")
	}
	if data.Content != "报销单已提交" {
		t.Errorf("内容应为'报销单已提交'，实际为: %s", data.Content)
	}
	if data.Delta {
		t.Error("delta 应为 false")
	}
}

// TestNewPhaseChangeEvent 验证 phase_change 事件的结构正确
func TestNewPhaseChangeEvent(t *testing.T) {
	event := agent.NewPhaseChangeEvent(
		"phase1_collect",
		"phase2_validate",
		"已收集 3 张票据，总金额 450.00 元，进入校验阶段",
	)

	if event.Type != agent.EventTypePhaseChange {
		t.Errorf("事件类型应为 phase_change，实际为: %s", event.Type)
	}

	data, ok := event.Data.(agent.PhaseChangeData)
	if !ok {
		t.Fatal("Data 应为 PhaseChangeData 类型")
	}
	if data.From != "phase1_collect" {
		t.Errorf("From 应为'phase1_collect'，实际为: %s", data.From)
	}
	if data.To != "phase2_validate" {
		t.Errorf("To 应为'phase2_validate'，实际为: %s", data.To)
	}
	if data.Summary == "" {
		t.Error("Summary 不应为空")
	}
}

// TestNewConfirmRequiredEvent 验证 confirm_required 事件的结构正确
func TestNewConfirmRequiredEvent(t *testing.T) {
	ctx := map[string]any{"total": 450.00, "count": 3}
	event := agent.NewConfirmRequiredEvent(
		"请确认以上 3 张票据信息无误",
		"confirm_invoice",
		ctx,
	)

	if event.Type != agent.EventTypeConfirmRequired {
		t.Errorf("事件类型应为 confirm_required，实际为: %s", event.Type)
	}

	data, ok := event.Data.(agent.ConfirmRequiredData)
	if !ok {
		t.Fatal("Data 应为 ConfirmRequiredData 类型")
	}
	if data.Prompt == "" {
		t.Error("Prompt 不应为空")
	}
	if data.Action != "confirm_invoice" {
		t.Errorf("Action 应为'confirm_invoice'，实际为: %s", data.Action)
	}
}

// TestNewErrorEvent 验证 error 事件的结构正确
func TestNewErrorEvent(t *testing.T) {
	event := agent.NewErrorEvent("OCR 识别失败，请手动输入金额", true, "OCR_FAILED")

	if event.Type != agent.EventTypeError {
		t.Errorf("事件类型应为 error，实际为: %s", event.Type)
	}

	data, ok := event.Data.(agent.ErrorData)
	if !ok {
		t.Fatal("Data 应为 ErrorData 类型")
	}
	if data.Message != "OCR 识别失败，请手动输入金额" {
		t.Errorf("消息内容不匹配，实际为: %s", data.Message)
	}
	if !data.Retry {
		t.Error("retry 应为 true")
	}
	if data.Code != "OCR_FAILED" {
		t.Errorf("Code 应为'OCR_FAILED'，实际为: %s", data.Code)
	}
}

// TestNewErrorEvent_NotRetryable 验证不可重试的错误事件
func TestNewErrorEvent_NotRetryable(t *testing.T) {
	event := agent.NewErrorEvent("系统内部错误", false, "INTERNAL_ERROR")

	data, ok := event.Data.(agent.ErrorData)
	if !ok {
		t.Fatal("Data 应为 ErrorData 类型")
	}
	if data.Retry {
		t.Error("retry 应为 false")
	}
}

// TestNewDoneEvent 验证 done 事件的结构正确
func TestNewDoneEvent(t *testing.T) {
	event := agent.NewDoneEvent()

	if event.Type != agent.EventTypeDone {
		t.Errorf("事件类型应为 done，实际为: %s", event.Type)
	}
	if event.Data != nil {
		t.Errorf("done 事件的 Data 应为 nil，实际为: %v", event.Data)
	}
}

// ============================================
// GinSSEWriter 测试
// ============================================

// TestGinSSEWriter_NewWithFlusher 验证使用正常的 gin context 创建 SSE Writer 成功
func TestGinSSEWriter_NewWithFlusher(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writer, err := agent.NewGinSSEWriter(c)

	if err != nil {
		t.Fatalf("创建 GinSSEWriter 失败: %v", err)
	}
	if writer == nil {
		t.Fatal("writer 不应为 nil")
	}

	// 验证 Content-Type 响应头已设置
	contentType := w.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("Content-Type 应为'text/event-stream'，实际为: %s", contentType)
	}

	// 验证 Cache-Control 响应头
	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl != "no-cache" {
		t.Errorf("Cache-Control 应为'no-cache'，实际为: %s", cacheControl)
	}
}

// TestGinSSEWriter_NotFlusher 验证 Writer 不支持 Flusher 时返回错误
func TestGinSSEWriter_NotFlusher(t *testing.T) {
	// 创建一个 gin.Context 但不初始化 Writer（nil 接口值），
	// 此时 c.Writer.(http.Flusher) 的类型断言将失败
	c := &gin.Context{}

	writer, err := agent.NewGinSSEWriter(c)

	if err == nil {
		t.Error("Writer 不支持 Flusher 时应返回错误")
	}
	if writer != nil {
		t.Error("失败时 writer 应为 nil")
	}
}

// TestGinSSEWriter_WriteEvent 验证 WriteEvent 写入正确的 SSE 格式
func TestGinSSEWriter_WriteEvent(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writer, err := agent.NewGinSSEWriter(c)
	if err != nil {
		t.Fatalf("创建 GinSSEWriter 失败: %v", err)
	}

	event := agent.NewThinkingEvent("正在处理...")
	err = writer.WriteEvent(event)
	if err != nil {
		t.Fatalf("WriteEvent 失败: %v", err)
	}

	// 检查写入的内容
	body := w.Body.String()
	if !strings.Contains(body, "event: thinking") {
		t.Error("SSE 输出应包含'event: thinking'")
	}
	if !strings.Contains(body, "data:") {
		t.Error("SSE 输出应包含'data:'")
	}
	if !strings.Contains(body, "正在处理...") {
		t.Error("SSE 输出应包含事件消息内容'正在处理...'")
	}
}

// TestGinSSEWriter_WriteEvent_Flush 验证 WriteEvent 后可以 Flush
func TestGinSSEWriter_WriteEvent_Flush(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writer, err := agent.NewGinSSEWriter(c)
	if err != nil {
		t.Fatalf("创建 GinSSEWriter 失败: %v", err)
	}

	event := agent.NewMessageEvent("测试消息", false)
	if err := writer.WriteEvent(event); err != nil {
		t.Fatalf("WriteEvent 失败: %v", err)
	}

	// Flush 不应报错
	if err := writer.Flush(); err != nil {
		t.Errorf("Flush 不应失败: %v", err)
	}
}

// TestGinSSEWriter_Close 验证 Close 不报错
func TestGinSSEWriter_Close(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writer, err := agent.NewGinSSEWriter(c)
	if err != nil {
		t.Fatalf("创建 GinSSEWriter 失败: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Errorf("Close 不应失败: %v", err)
	}
}

// TestGinSSEWriter_WriteMultipleEvents 验证写入多个事件时内容正确累积
func TestGinSSEWriter_WriteMultipleEvents(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writer, err := agent.NewGinSSEWriter(c)
	if err != nil {
		t.Fatalf("创建 GinSSEWriter 失败: %v", err)
	}

	writer.WriteEvent(agent.NewThinkingEvent("第一步"))
	writer.WriteEvent(agent.NewMessageEvent("结果", false))
	writer.WriteEvent(agent.NewDoneEvent())

	body := w.Body.String()

	if !strings.Contains(body, "第一步") {
		t.Error("输出应包含第一个事件的消息")
	}
	if !strings.Contains(body, "结果") {
		t.Error("输出应包含第二个事件的消息")
	}
	if !strings.Contains(body, "event: done") {
		t.Error("输出应包含 done 事件")
	}

	// 验证事件之间有空行分隔
	if !strings.Contains(body, "\n\n") {
		t.Error("SSE 事件之间应有空行分隔")
	}
}

// TestGinSSEWriter_ConnectionHeaders 验证 SSE 连接响应头设置完整
func TestGinSSEWriter_ConnectionHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writer, err := agent.NewGinSSEWriter(c)
	if err != nil {
		t.Fatalf("创建 GinSSEWriter 失败: %v", err)
	}
	_ = writer

	if w.Header().Get("Connection") != "keep-alive" {
		t.Errorf("Connection 响应头应为'keep-alive'，实际为: %s", w.Header().Get("Connection"))
	}
	if w.Header().Get("X-Accel-Buffering") != "no" {
		t.Errorf("X-Accel-Buffering 响应头应为'no'，实际为: %s", w.Header().Get("X-Accel-Buffering"))
	}
}

// TestNewMessageEvent_EmptyContent 验证空消息事件
func TestNewMessageEvent_EmptyContent(t *testing.T) {
	event := agent.NewMessageEvent("", false)

	data, ok := event.Data.(agent.MessageData)
	if !ok {
		t.Fatal("Data 应为 MessageData 类型")
	}
	if data.Content != "" {
		t.Errorf("空消息的 Content 应为空，实际为: %s", data.Content)
	}
}

// TestNewThinkingEvent_EmptyMessage 验证空消息的 thinking 事件
func TestNewThinkingEvent_EmptyMessage(t *testing.T) {
	event := agent.NewThinkingEvent("")

	data, ok := event.Data.(agent.ThinkingData)
	if !ok {
		t.Fatal("Data 应为 ThinkingData 类型")
	}
	if data.Message != "" {
		t.Errorf("空消息的 Message 应为空，实际为: %s", data.Message)
	}
}
