package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ============================================
// Mock 实现
// ============================================

// mockSSEWriter 捕获 SSE 事件用于测试验证
type mockSSEWriter struct {
	events []agent.SSEEvent
	flushCount int
}

func (m *mockSSEWriter) WriteEvent(event agent.SSEEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockSSEWriter) Flush() error {
	m.flushCount++
	return nil
}

// hasEventType 检查事件列表中是否包含指定类型的事件
func (m *mockSSEWriter) hasEventType(typ agent.SSEEventType) bool {
	for _, e := range m.events {
		if e.Type == typ {
			return true
		}
	}
	return false
}

// mockSessionStore 实现 infra.SessionStore 接口，用于测试会话持久化
type mockSessionStore struct {
	history    []*schema.Message
	historyErr error

	saved map[string][]*schema.Message // sessionID → 已保存消息
	saveErr error

	clearErr error

	states   map[string]any
	stateErr error
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{
		saved:  make(map[string][]*schema.Message),
		states: make(map[string]any),
	}
}

func (m *mockSessionStore) SaveMessages(ctx context.Context, sessionID string, msgs []*schema.Message) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved[sessionID] = append(m.saved[sessionID], msgs...)
	return nil
}

func (m *mockSessionStore) GetHistory(ctx context.Context, sessionID string, limit int) ([]*schema.Message, error) {
	if m.historyErr != nil {
		return nil, m.historyErr
	}
	return m.history, nil
}

func (m *mockSessionStore) Clear(ctx context.Context, sessionID string) error {
	return m.clearErr
}

func (m *mockSessionStore) SaveState(ctx context.Context, sessionID string, key string, state any) error {
	m.states[sessionID+":"+key] = state
	return m.stateErr
}

func (m *mockSessionStore) GetState(ctx context.Context, sessionID string, key string, target any) (bool, error) {
	v, ok := m.states[sessionID+":"+key]
	if !ok {
		return false, nil
	}
	data, _ := json.Marshal(v)
	json.Unmarshal(data, target)
	return true, m.stateErr
}

func (m *mockSessionStore) DeleteState(ctx context.Context, sessionID string, key string) error {
	delete(m.states, sessionID+":"+key)
	return nil
}

// mockCheckpointStore 实现 agent.CheckpointStore 接口
type mockCheckpointStore struct {
	data      map[string][]byte
	getErr    error
	setErr    error
	deleteErr error
}

func newMockCheckpointStore() *mockCheckpointStore {
	return &mockCheckpointStore{
		data: make(map[string][]byte),
	}
}

func (m *mockCheckpointStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if m.getErr != nil {
		return nil, false, m.getErr
	}
	d, ok := m.data[key]
	return d, ok, nil
}

func (m *mockCheckpointStore) Set(ctx context.Context, key string, value []byte) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.data[key] = value
	return nil
}

func (m *mockCheckpointStore) Delete(ctx context.Context, key string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.data, key)
	return nil
}

// mockRunnable 实现 compose.Runnable[agent.AgentInput, *schema.Message]，用于模拟 Graph 行为
// 仅 Invoke 被 StreamChat 实际使用，其余方法返回"未实现"错误
type mockRunnable struct {
	invokeFunc func(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error)
}

func (m *mockRunnable) Invoke(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error) {
	return m.invokeFunc(ctx, input, opts...)
}

func (m *mockRunnable) Stream(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream 未实现")
}

func (m *mockRunnable) Collect(ctx context.Context, input *schema.StreamReader[agent.AgentInput], opts ...compose.Option) (*schema.Message, error) {
	return nil, errors.New("collect 未实现")
}

func (m *mockRunnable) Transform(ctx context.Context, input *schema.StreamReader[agent.AgentInput], opts ...compose.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("transform 未实现")
}

// ============================================
// 测试辅助函数
// ============================================

// testLogger 创建测试用日志器（静默输出）
func testLogger(t *testing.T) *log.Logger {
	t.Helper()
	return &log.Logger{Logger: zap.NewNop()}
}

// defaultConfig 创建测试用 AgentConfig
func defaultConfig() *agent.AgentConfig {
	return &agent.AgentConfig{
		SessionTTLMinutes:         30,
		MaxHistoryTurns:           5,
		MaxPhaseTurns:             10,
		CheckpointCleanupHours:    1,
		LLMMaxRetries:             3,
		LLMRetryBackoffSeconds:    2,
		ToolTimeoutSeconds:        30,
		IntentConfidenceThreshold: 0.7,
	}
}

// validInput 创建一个有效的 AgentInput 用于测试
func validInput() agent.AgentInput {
	return agent.AgentInput{
		SessionID:  "test-session-001",
		UserID:     42,
		EmployeeID: "EMP001",
		Role:       "employee",
		Message:    "帮我报销一张发票",
	}
}

// ============================================
// TestAgentRunner_StreamChat_Success
// ============================================

func TestAgentRunner_StreamChat_Success(t *testing.T) {
	// 准备：创建 mock graph，返回"你好，我是 Reimbee"
	mockGraph := &mockRunnable{
		invokeFunc: func(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error) {
			return schema.AssistantMessage("你好，我是 Reimbee，很高兴为你服务！", nil), nil
		},
	}

	sseWriter := &mockSSEWriter{}
	sessionStore := newMockSessionStore()
	checkpointStore := newMockCheckpointStore()
	config := defaultConfig()
	logger := testLogger(t)

	runner := agent.NewAgentRunner(mockGraph, sessionStore, checkpointStore, config, logger)
	input := validInput()

	// 执行
	err := runner.StreamChat(context.Background(), input, sseWriter)

	// 验证：无错误
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}

	// 验证：SSE 事件包含 thinking + message + done（按顺序）
	if len(sseWriter.events) < 3 {
		t.Fatalf("期望至少 3 个 SSE 事件，实际: %d", len(sseWriter.events))
	}

	if sseWriter.events[0].Type != agent.EventTypeThinking {
		t.Errorf("第一个事件期望 thinking，实际: %s", sseWriter.events[0].Type)
	}
	if sseWriter.events[1].Type != agent.EventTypeMessage {
		t.Errorf("第二个事件期望 message，实际: %s", sseWriter.events[1].Type)
	}
	if sseWriter.events[len(sseWriter.events)-1].Type != agent.EventTypeDone {
		t.Errorf("最后一个事件期望 done，实际: %s", sseWriter.events[len(sseWriter.events)-1].Type)
	}

	// 验证：message 事件包含正确内容
	msgData, ok := sseWriter.events[1].Data.(agent.MessageData)
	if !ok {
		t.Fatalf("message 事件的 Data 类型错误，期望 agent.MessageData")
	}
	if msgData.Content != "你好，我是 Reimbee，很高兴为你服务！" {
		t.Errorf("消息内容不匹配，期望 '你好，我是 Reimbee，很高兴为你服务！'，实际: '%s'", msgData.Content)
	}

	// 验证：done 事件的 delta 标记为 false（非流式增量）
	if msgData.Delta {
		t.Error("期望 message 事件的 Delta 为 false")
	}

	// 验证：会话存储被调用——保存了用户消息和 assistant 消息
	saved := sessionStore.saved[input.SessionID]
	if len(saved) != 2 {
		t.Fatalf("期望保存 2 条消息，实际: %d", len(saved))
	}

	// 第一条是用户消息
	if saved[0].Role != schema.User {
		t.Errorf("第一条消息期望 user，实际: %s", saved[0].Role)
	}
	if saved[0].Content != input.Message {
		t.Errorf("用户消息内容不匹配，期望 '%s'，实际: '%s'", input.Message, saved[0].Content)
	}

	// 第二条是 assistant 消息
	if saved[1].Role != schema.Assistant {
		t.Errorf("第二条消息期望 assistant，实际: %s", saved[1].Role)
	}
	if saved[1].Content != "你好，我是 Reimbee，很高兴为你服务！" {
		t.Errorf("assistant 消息内容不匹配，实际: '%s'", saved[1].Content)
	}

	// 验证：Flush 被调用多次（每个事件后至少一次）
	if sseWriter.flushCount < len(sseWriter.events) {
		t.Errorf("Flush 调用次数不足: %d < 事件数 %d", sseWriter.flushCount, len(sseWriter.events))
	}
}

// ============================================
// TestAgentRunner_StreamChat_GraphError
// ============================================

func TestAgentRunner_StreamChat_GraphError(t *testing.T) {
	graphErr := errors.New("LLM 服务暂时不可用")
	mockGraph := &mockRunnable{
		invokeFunc: func(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error) {
			return nil, graphErr
		},
	}

	sseWriter := &mockSSEWriter{}
	sessionStore := newMockSessionStore()
	checkpointStore := newMockCheckpointStore()
	config := defaultConfig()
	logger := testLogger(t)

	runner := agent.NewAgentRunner(mockGraph, sessionStore, checkpointStore, config, logger)
	input := validInput()

	// 执行
	err := runner.StreamChat(context.Background(), input, sseWriter)

	// 验证：返回错误
	if err == nil {
		t.Fatal("期望返回错误，实际为 nil")
	}
	if !strings.Contains(err.Error(), "Graph执行失败") {
		t.Errorf("期望错误包含 'Graph执行失败'，实际: %v", err)
	}
	if !strings.Contains(err.Error(), graphErr.Error()) {
		t.Errorf("期望错误包含原始错误消息，实际: %v", err)
	}

	// 验证：thinking 事件已发送
	if !sseWriter.hasEventType(agent.EventTypeThinking) {
		t.Error("期望收到 thinking 事件")
	}

	// 验证：error 事件已发送
	hasError := false
	for _, e := range sseWriter.events {
		if e.Type == agent.EventTypeError {
			hasError = true
			errData, ok := e.Data.(agent.ErrorData)
			if !ok {
				t.Errorf("error 事件的 Data 类型错误")
				continue
			}
			if !strings.Contains(errData.Message, "LLM 服务暂时不可用") {
				t.Errorf("错误事件消息不包含预期内容，实际: %s", errData.Message)
			}
			if !errData.Retry {
				t.Error("期望 error 事件的 Retry 为 true")
			}
			if errData.Code != "graph_error" {
				t.Errorf("期望错误代码 'graph_error'，实际: '%s'", errData.Code)
			}
		}
	}
	if !hasError {
		t.Error("期望收到 error 事件")
	}

	// 验证：Graph 出错时不应发送 done 事件
	if sseWriter.hasEventType(agent.EventTypeDone) {
		t.Error("Graph 出错时不应发送 done 事件")
	}

	// 验证：用户消息仍被保存（在 Graph 调用之前）
	saved := sessionStore.saved[input.SessionID]
	if len(saved) != 1 {
		t.Fatalf("Graph 出错后用户消息仍应被保存，期望 1 条，实际: %d", len(saved))
	}
	if saved[0].Role != schema.User {
		t.Errorf("期望 user 消息，实际: %s", saved[0].Role)
	}
}

// ============================================
// TestAgentRunner_StreamChat_EmptyInput
// ============================================

func TestAgentRunner_StreamChat_EmptyInput(t *testing.T) {
	// 空消息场景：Graph 返回空的 assistant 消息
	mockGraph := &mockRunnable{
		invokeFunc: func(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error) {
			return schema.AssistantMessage("", nil), nil
		},
	}

	sseWriter := &mockSSEWriter{}
	sessionStore := newMockSessionStore()
	checkpointStore := newMockCheckpointStore()
	config := defaultConfig()
	logger := testLogger(t)

	runner := agent.NewAgentRunner(mockGraph, sessionStore, checkpointStore, config, logger)

	emptyInput := agent.AgentInput{
		SessionID:  "empty-session",
		UserID:     1,
		EmployeeID: "EMP001",
		Role:       "employee",
		Message:    "",
	}

	// 执行
	err := runner.StreamChat(context.Background(), emptyInput, sseWriter)

	// 验证：无错误（空输入不是错误，Graph 正常处理）
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}

	// 验证：thinking + done 事件存在（message 事件因 content 为空被跳过）
	if !sseWriter.hasEventType(agent.EventTypeThinking) {
		t.Error("期望收到 thinking 事件")
	}
	if !sseWriter.hasEventType(agent.EventTypeDone) {
		t.Error("期望收到 done 事件")
	}

	// 验证：message 事件未发送（因为 Content 为空）
	if sseWriter.hasEventType(agent.EventTypeMessage) {
		t.Error("空 content 时不应发送 message 事件")
	}

	// 验证：用户消息仍被保存
	saved := sessionStore.saved[emptyInput.SessionID]
	if len(saved) != 1 {
		t.Fatalf("期望保存 1 条用户消息，实际: %d", len(saved))
	}
}

// ============================================
// TestAgentRunner_StreamChat_HistoryError
// ============================================

func TestAgentRunner_StreamChat_HistoryError(t *testing.T) {
	// 场景：获取历史消息失败，应降级为空历史继续执行
	mockGraph := &mockRunnable{
		invokeFunc: func(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error) {
			return schema.AssistantMessage("历史加载失败，但我仍能回复", nil), nil
		},
	}

	sseWriter := &mockSSEWriter{}
	sessionStore := newMockSessionStore()
	sessionStore.historyErr = errors.New("数据库连接超时")
	checkpointStore := newMockCheckpointStore()
	config := defaultConfig()
	logger := testLogger(t)

	runner := agent.NewAgentRunner(mockGraph, sessionStore, checkpointStore, config, logger)
	input := validInput()

	// 执行
	err := runner.StreamChat(context.Background(), input, sseWriter)

	// 验证：历史失败不应导致整体失败（降级策略）
	if err != nil {
		t.Fatalf("历史错误应被降级处理，不应返回错误，实际: %v", err)
	}

	// 验证：流程正常完成，收到 done 事件
	if !sseWriter.hasEventType(agent.EventTypeDone) {
		t.Error("历史错误降级后仍应正常完成")
	}
}

// ============================================
// TestAgentRunner_StreamChat_SaveUserMessageError
// ============================================

func TestAgentRunner_StreamChat_SaveUserMessageError(t *testing.T) {
	// 场景：保存用户消息失败，不应中断流程（非致命错误）
	mockGraph := &mockRunnable{
		invokeFunc: func(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error) {
			return schema.AssistantMessage("保存失败但我继续工作", nil), nil
		},
	}

	sseWriter := &mockSSEWriter{}
	sessionStore := newMockSessionStore()
	sessionStore.saveErr = errors.New("Redis 不可用")
	checkpointStore := newMockCheckpointStore()
	config := defaultConfig()
	logger := testLogger(t)

	runner := agent.NewAgentRunner(mockGraph, sessionStore, checkpointStore, config, logger)
	input := validInput()

	// 执行
	err := runner.StreamChat(context.Background(), input, sseWriter)

	// 验证：保存失败不阻塞流程
	if err != nil {
		t.Fatalf("保存消息失败应降级处理，不应返回错误，实际: %v", err)
	}

	// 验证：done 事件仍然发送
	if !sseWriter.hasEventType(agent.EventTypeDone) {
		t.Error("期望在保存失败时仍发送 done 事件")
	}
}

// ============================================
// TestAgentRunner_StreamChat_GraphReturnsNil
// ============================================

func TestAgentRunner_StreamChat_GraphReturnsNil(t *testing.T) {
	// 场景：Graph 返回 nil 消息（非错误场景）
	mockGraph := &mockRunnable{
		invokeFunc: func(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error) {
			return nil, nil
		},
	}

	sseWriter := &mockSSEWriter{}
	sessionStore := newMockSessionStore()
	checkpointStore := newMockCheckpointStore()
	config := defaultConfig()
	logger := testLogger(t)

	runner := agent.NewAgentRunner(mockGraph, sessionStore, checkpointStore, config, logger)
	input := validInput()

	// 执行
	err := runner.StreamChat(context.Background(), input, sseWriter)

	// 验证：无错误
	if err != nil {
		t.Fatalf("期望无错误，实际: %v", err)
	}

	// 验证：nil 消息不触发 message 或 assistant 保存事件
	if sseWriter.hasEventType(agent.EventTypeMessage) {
		t.Error("Graph 返回 nil 时不应发送 message 事件")
	}

	// done 事件仍然发送
	if !sseWriter.hasEventType(agent.EventTypeDone) {
		t.Error("期望发送 done 事件")
	}

	// 仅保存了用户消息（assistant 为 nil 不保存）
	saved := sessionStore.saved[input.SessionID]
	if len(saved) != 1 {
		t.Fatalf("期望仅保存 1 条消息（用户消息），实际: %d", len(saved))
	}
}

// ============================================
// TestBuildAgentInput
// ============================================

func TestBuildAgentInput(t *testing.T) {
	sessionID := "sess-abc-123"
	message := "我要报销一张餐费发票"
	employeeID := "EMP0042"
	var userID uint = 99
	role := "employee"

	input := agent.BuildAgentInput(sessionID, message, employeeID, userID, role)

	if input.SessionID != sessionID {
		t.Errorf("SessionID 不匹配，期望 '%s'，实际: '%s'", sessionID, input.SessionID)
	}
	if input.Message != message {
		t.Errorf("Message 不匹配，期望 '%s'，实际: '%s'", message, input.Message)
	}
	if input.EmployeeID != employeeID {
		t.Errorf("EmployeeID 不匹配，期望 '%s'，实际: '%s'", employeeID, input.EmployeeID)
	}
	if input.UserID != userID {
		t.Errorf("UserID 不匹配，期望 %d，实际: %d", userID, input.UserID)
	}
	if input.Role != role {
		t.Errorf("Role 不匹配，期望 '%s'，实际: '%s'", role, input.Role)
	}
}

// ============================================
// TestBuildAgentInput_EmptyValues
// ============================================

func TestBuildAgentInput_EmptyValues(t *testing.T) {
	// 验证空字符串/零值输入不会 panic
	input := agent.BuildAgentInput("", "", "", 0, "")

	if input.SessionID != "" {
		t.Errorf("SessionID 期望空字符串，实际: '%s'", input.SessionID)
	}
	if input.Message != "" {
		t.Errorf("Message 期望空字符串，实际: '%s'", input.Message)
	}
	if input.UserID != 0 {
		t.Errorf("UserID 期望 0，实际: %d", input.UserID)
	}
}

// ============================================
// TestAgentRunner_NewAgentRunner
// ============================================

func TestAgentRunner_NewAgentRunner(t *testing.T) {
	// 验证 NewAgentRunner 返回非 nil 实例
	mockGraph := &mockRunnable{
		invokeFunc: func(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error) {
			return schema.AssistantMessage("hello", nil), nil
		},
	}
	sessionStore := newMockSessionStore()
	checkpointStore := newMockCheckpointStore()
	config := defaultConfig()
	logger := testLogger(t)

	runner := agent.NewAgentRunner(mockGraph, sessionStore, checkpointStore, config, logger)

	if runner == nil {
		t.Fatal("NewAgentRunner 返回 nil")
	}
}

// ============================================
// TestAgentRunner_StreamChat_SessionMessageIsolation
// ============================================

func TestAgentRunner_StreamChat_SessionMessageIsolation(t *testing.T) {
	// 验证不同 session 之间的消息保存互不干扰
	mockGraph := &mockRunnable{
		invokeFunc: func(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error) {
			return schema.AssistantMessage("re: "+input.Message, nil), nil
		},
	}

	sessionStore := newMockSessionStore()
	checkpointStore := newMockCheckpointStore()
	config := defaultConfig()
	logger := testLogger(t)

	runner := agent.NewAgentRunner(mockGraph, sessionStore, checkpointStore, config, logger)

	// 第一个会话
	input1 := agent.AgentInput{
		SessionID:  "session-A",
		UserID:     1,
		EmployeeID: "E1",
		Role:       "employee",
		Message:    "消息A",
	}
	sseWriter1 := &mockSSEWriter{}
	err := runner.StreamChat(context.Background(), input1, sseWriter1)
	if err != nil {
		t.Fatalf("session-A 执行失败: %v", err)
	}

	// 第二个会话
	input2 := agent.AgentInput{
		SessionID:  "session-B",
		UserID:     2,
		EmployeeID: "E2",
		Role:       "approver",
		Message:    "消息B",
	}
	sseWriter2 := &mockSSEWriter{}
	err = runner.StreamChat(context.Background(), input2, sseWriter2)
	if err != nil {
		t.Fatalf("session-B 执行失败: %v", err)
	}

	// 验证：两个 session 各自只保存了自己的消息
	savedA := sessionStore.saved["session-A"]
	savedB := sessionStore.saved["session-B"]

	if len(savedA) != 2 {
		t.Errorf("session-A 期望 2 条消息，实际: %d", len(savedA))
	}
	if len(savedB) != 2 {
		t.Errorf("session-B 期望 2 条消息，实际: %d", len(savedB))
	}

	// session-A 不应包含 session-B 的消息
	for _, msg := range savedA {
		if strings.Contains(msg.Content, "消息B") {
			t.Error("session-A 中不应包含 session-B 的消息")
		}
	}
	for _, msg := range savedB {
		if strings.Contains(msg.Content, "消息A") {
			t.Error("session-B 中不应包含 session-A 的消息")
		}
	}
}

// ============================================
// TestAgentRunner_StreamChat_ThinkingEventContent
// ============================================

func TestAgentRunner_StreamChat_ThinkingEventContent(t *testing.T) {
	// 验证 thinking 事件的负载内容正确
	mockGraph := &mockRunnable{
		invokeFunc: func(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error) {
			return schema.AssistantMessage("回答内容", nil), nil
		},
	}

	sseWriter := &mockSSEWriter{}
	sessionStore := newMockSessionStore()
	checkpointStore := newMockCheckpointStore()
	config := defaultConfig()
	logger := testLogger(t)

	runner := agent.NewAgentRunner(mockGraph, sessionStore, checkpointStore, config, logger)

	err := runner.StreamChat(context.Background(), validInput(), sseWriter)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	// 验证 thinking 事件
	if len(sseWriter.events) == 0 {
		t.Fatal("没有收到任何事件")
	}

	thinkingEvent := sseWriter.events[0]
	if thinkingEvent.Type != agent.EventTypeThinking {
		t.Fatalf("第一个事件不是 thinking，实际: %s", thinkingEvent.Type)
	}

	thinkingData, ok := thinkingEvent.Data.(agent.ThinkingData)
	if !ok {
		t.Fatalf("thinking Data 类型错误：%T", thinkingEvent.Data)
	}

	if thinkingData.Message != "正在理解您的需求..." {
		t.Errorf("thinking 消息不匹配，期望 '正在理解您的需求...'，实际: '%s'", thinkingData.Message)
	}
}
