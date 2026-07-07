// Package agent_test LoopManager 黑盒测试
// 测试 GetOrCreate、PushMessage、Shutdown 核心生命周期方法
package agent_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ============================================
// mockSessionStore — 内存实现 infra.SessionStore
// ============================================

type mockSessionStore struct {
	mu       sync.Mutex
	messages map[string][]*schema.Message
	states   map[string]map[string][]byte
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{
		messages: make(map[string][]*schema.Message),
		states:   make(map[string]map[string][]byte),
	}
}

func (m *mockSessionStore) SaveMessages(_ context.Context, sessionID string, msgs []*schema.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range msgs {
		// 深度拷贝：JSON 序列化再反序列化，模拟 MySQL 存储语义
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		var copyMsg schema.Message
		if err := json.Unmarshal(data, &copyMsg); err != nil {
			return err
		}
		m.messages[sessionID] = append(m.messages[sessionID], &copyMsg)
	}
	return nil
}

func (m *mockSessionStore) GetHistory(_ context.Context, sessionID string, limit int) ([]*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.messages[sessionID]
	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	if len(all) == 0 {
		return nil, nil
	}
	// 返回最近 limit 条消息（按时间正序，最新的在后）
	start := max(len(all)-limit, 0)
	result := make([]*schema.Message, limit)
	for i := 0; i < limit; i++ {
		data, _ := json.Marshal(all[start+i])
		var msg schema.Message
		_ = json.Unmarshal(data, &msg)
		result[i] = &msg
	}
	return result, nil
}

func (m *mockSessionStore) Clear(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.messages, sessionID)
	return nil
}

func (m *mockSessionStore) SaveState(_ context.Context, sessionID string, key string, state any) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.states[sessionID] == nil {
		m.states[sessionID] = make(map[string][]byte)
	}
	m.states[sessionID][key] = data
	return nil
}

func (m *mockSessionStore) GetState(_ context.Context, sessionID string, key string, target any) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := m.states[sessionID]
	if entries == nil {
		return false, nil
	}
	data, ok := entries[key]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(data, target); err != nil {
		return false, err
	}
	return true, nil
}

func (m *mockSessionStore) DeleteState(_ context.Context, sessionID string, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.states[sessionID] != nil {
		delete(m.states[sessionID], key)
	}
	return nil
}

// 编译期验证 mockSessionStore 实现了 infra.SessionStore 接口
var _ infra.SessionStore = (*mockSessionStore)(nil)

// ============================================
// mockSSEWriter — 实现 agent.SSEWriter（用于 PushMessage 测试）
// ============================================

type mockSSEWriter struct {
	mu     sync.Mutex
	events []agent.SSEEvent
}

func newMockSSEWriter() *mockSSEWriter {
	return &mockSSEWriter{}
}

func (w *mockSSEWriter) WriteEvent(event agent.SSEEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, event)
	return nil
}

func (w *mockSSEWriter) Flush() error {
	return nil
}

// Events 返回所有已写入的事件
func (w *mockSSEWriter) Events() []agent.SSEEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]agent.SSEEvent, len(w.events))
	copy(result, w.events)
	return result
}

// ============================================
// mockInvokableTool — 实现 tool.InvokableTool
// ============================================

type mockInvokableTool struct{ name string }

func (m *mockInvokableTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: m.name}, nil
}

func (m *mockInvokableTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	return `{"status":"ok"}`, nil
}

// ============================================
// 测试辅助函数
// ============================================

func testLogger(t *testing.T) *log.Logger {
	t.Helper()
	return &log.Logger{Logger: zap.NewNop()}
}

// newTestToolSet 创建带有 mock 工具的 ToolSet，用于 LoopManager 初始化
func newTestToolSet(t *testing.T) *tools.ToolSet {
	t.Helper()
	logger := testLogger(t)
	mt := &mockInvokableTool{name: "test_tool"}
	return tools.NewToolSet(
		&tools.OCRTool{InvokableTool: mt},
		&tools.ComplianceTool{InvokableTool: mt},
		&tools.BudgetTool{InvokableTool: mt},
		&tools.PDFTool{InvokableTool: mt},
		&tools.EmailTool{InvokableTool: mt},
		&tools.ProgressTool{InvokableTool: mt},
		&tools.QueryTool{InvokableTool: mt},
		&tools.ConfirmInvoiceTool{InvokableTool: mt},
		&tools.ConfirmSubmitTool{InvokableTool: mt},
		&tools.CreateReimbTool{InvokableTool: mt},
		&tools.SubmitReimbTool{InvokableTool: mt},
		nil, // SessionStore（NewToolSet v3.0 参数，测试中传 nil 即可）
		logger,
	)
}

// newTestLoopManager 创建最小可工作的 LoopManager
// 使用 MockChatModel + mock SessionStore + mock ToolSet
func newTestLoopManager(t *testing.T) *agent.LoopManager {
	t.Helper()
	store := newMockSessionStore()
	chatModel := testutil.NewTextReplyChatModel("收到，正在处理您的请求。")
	toolSet := newTestToolSet(t)
	logger := testLogger(t)
	cfg := &agent.LoopConfig{
		SessionTTL:      30 * 60 * 60, // 极长 TTL，测试期间不自动清理
		MaxHistoryTurns: 10,
		CleanupInterval: 60 * 60, // 极长间隔
	}

	// NewLoopManager 通过 Wire 自动注入，函数签名：
	// func NewLoopManager(store, chatModel, toolSet, logger, config)
	mgr := agent.NewLoopManager(store, chatModel, toolSet, logger, cfg)
	return mgr
}

// ============================================
// GetOrCreate 测试
// ============================================

// TestLoopManager_GetOrCreate_Create 验证首次调用创建新 SessionLoop
func TestLoopManager_GetOrCreate_Create(t *testing.T) {
	mgr := newTestLoopManager(t)
	defer mgr.Shutdown()

	sl := mgr.GetOrCreate("session-1")
	if sl == nil {
		t.Fatal("GetOrCreate 返回 nil，期望非 nil SessionLoop")
	}
}

// TestLoopManager_GetOrCreate_Reuse 验证同一 sessionID 返回相同 SessionLoop
func TestLoopManager_GetOrCreate_Reuse(t *testing.T) {
	mgr := newTestLoopManager(t)
	defer mgr.Shutdown()

	sl1 := mgr.GetOrCreate("session-2")
	sl2 := mgr.GetOrCreate("session-2")

	if sl1 == nil || sl2 == nil {
		t.Fatal("GetOrCreate 返回 nil")
	}
	if sl1 != sl2 {
		t.Error("同一 sessionID 的两次 GetOrCreate 应返回相同 SessionLoop 指针")
	}
}

// TestLoopManager_GetOrCreate_MultiSession 验证不同 sessionID 创建不同的 SessionLoop
func TestLoopManager_GetOrCreate_MultiSession(t *testing.T) {
	mgr := newTestLoopManager(t)
	defer mgr.Shutdown()

	sl1 := mgr.GetOrCreate("session-a")
	sl2 := mgr.GetOrCreate("session-b")

	if sl1 == nil || sl2 == nil {
		t.Fatal("GetOrCreate 返回 nil")
	}
	if sl1 == sl2 {
		t.Error("不同 sessionID 应返回不同的 SessionLoop 实例")
	}
}

// ============================================
// PushMessage 测试
// ============================================

// TestLoopManager_PushMessage 验证消息推送注册 SSEWriter 且不 panic
func TestLoopManager_PushMessage(t *testing.T) {
	mgr := newTestLoopManager(t)
	defer mgr.Shutdown()

	writer := newMockSSEWriter()
	doneCh := make(chan error, 1)

	// PushMessage 不应 panic（内部调用 TurnLoop.Push）
	mgr.PushMessage("session-3", "你好，我要报销", writer, doneCh)
}

// TestLoopManager_PushMessage_NoWriter 验证 writer 为 nil 时不 panic
func TestLoopManager_PushMessage_NoWriter(t *testing.T) {
	mgr := newTestLoopManager(t)
	defer mgr.Shutdown()

	doneCh := make(chan error, 1)

	// PushMessage 即使 writer 为 nil 也不应 panic
	//（OnAgentEvents 回调会检测 writer==nil 并跳过输出）
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("PushMessage 触发 panic: %v", r)
			}
		}()
		mgr.PushMessage("session-4", "查询进度", nil, doneCh)
	}()
}

// ============================================
// Shutdown 测试
// ============================================

// TestLoopManager_Shutdown_NoPanic 验证 Shutdown 不 panic
func TestLoopManager_Shutdown_NoPanic(t *testing.T) {
	mgr := newTestLoopManager(t)

	// 先创建一个会话
	mgr.GetOrCreate("session-5")

	// Shutdown 不应 panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Shutdown 触发 panic: %v", r)
			}
		}()
		mgr.Shutdown()
	}()
}

// TestLoopManager_Shutdown_DoubleCall 验证重复 Shutdown 不 panic
func TestLoopManager_Shutdown_DoubleCall(t *testing.T) {
	mgr := newTestLoopManager(t)

	mgr.GetOrCreate("session-6")
	mgr.Shutdown()

	// 二次 Shutdown 不应 panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("二次 Shutdown 触发 panic: %v", r)
			}
		}()
		mgr.Shutdown()
	}()
}

// TestLoopManager_Shutdown_Empty 验证无活跃会话时 Shutdown 不 panic
func TestLoopManager_Shutdown_Empty(t *testing.T) {
	mgr := newTestLoopManager(t)

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("空会话 Shutdown 触发 panic: %v", r)
			}
		}()
		mgr.Shutdown()
	}()
}

// ============================================
// mockSessionStore 功能测试
// ============================================

// TestMockSessionStore_SaveAndGetMessages 验证 mock SessionStore 消息持久化
func TestMockSessionStore_SaveAndGetMessages(t *testing.T) {
	store := newMockSessionStore()
	ctx := context.Background()

	msg := schema.UserMessage("你好")
	err := store.SaveMessages(ctx, "s1", []*schema.Message{msg})
	if err != nil {
		t.Fatalf("SaveMessages 失败: %v", err)
	}

	history, err := store.GetHistory(ctx, "s1", 10)
	if err != nil {
		t.Fatalf("GetHistory 失败: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("期望 1 条消息，实际 %d 条", len(history))
	}
	if history[0].Content != "你好" {
		t.Errorf("消息内容 = %q, want %q", history[0].Content, "你好")
	}
}

// TestMockSessionStore_SaveAndGetState 验证 mock SessionStore 状态持久化
func TestMockSessionStore_SaveAndGetState(t *testing.T) {
	store := newMockSessionStore()
	ctx := context.Background()

	type testState struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	err := store.SaveState(ctx, "s1", "test_key", &testState{Name: "hello", Value: 42})
	if err != nil {
		t.Fatalf("SaveState 失败: %v", err)
	}

	var got testState
	found, err := store.GetState(ctx, "s1", "test_key", &got)
	if err != nil {
		t.Fatalf("GetState 失败: %v", err)
	}
	if !found {
		t.Fatal("期望状态存在，但 GetState 返回 false")
	}
	if got.Name != "hello" || got.Value != 42 {
		t.Errorf("状态 = %+v, want {Name:hello Value:42}", got)
	}
}

// TestMockSessionStore_Clear 验证 Clear 删除消息
func TestMockSessionStore_Clear(t *testing.T) {
	store := newMockSessionStore()
	ctx := context.Background()

	msg := schema.UserMessage("测试消息")
	_ = store.SaveMessages(ctx, "s1", []*schema.Message{msg})
	_ = store.Clear(ctx, "s1")

	history, _ := store.GetHistory(ctx, "s1", 10)
	if len(history) != 0 {
		t.Errorf("Clear 后期望 0 条消息，实际 %d 条", len(history))
	}
}

// TestMockSessionStore_DeleteState 验证 DeleteState 删除状态
func TestMockSessionStore_DeleteState(t *testing.T) {
	store := newMockSessionStore()
	ctx := context.Background()

	_ = store.SaveState(ctx, "s1", "k1", map[string]string{"a": "b"})
	_ = store.DeleteState(ctx, "s1", "k1")

	var target map[string]string
	found, _ := store.GetState(ctx, "s1", "k1", &target)
	if found {
		t.Error("DeleteState 后期望状态不存在，但 GetState 返回 true")
	}
}
