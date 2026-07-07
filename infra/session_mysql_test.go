package infra_test

import (
	"context"
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

func newTestStore(t *testing.T) (*infra.MySQLSessionStore, *infra.Data) {
	t.Helper()
	data := testutil.NewTestData()
	logger := &log.Logger{Logger: zap.NewNop()}
	metaRepo := infra.NewSessionMetaRepo(data.DB)
	store := infra.NewMySQLSessionStore(data, nil, metaRepo, logger)
	return store, data
}

// ── Session Meta CRUD ──

func TestSessionMeta_CreateAndGet(t *testing.T) {
	store, data := newTestStore(t)
	defer testutil.CleanDB(data)

	meta := &infra.SessionMeta{
		SessionID:  "test-session-1",
		UserID:     1,
		EmployeeID: "EMP001",
		Role:       "employee",
		Status:     infra.SessionStatusActive,
		Summary:    "测试会话",
	}
	if err := store.CreateSession(context.Background(), meta); err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	got, err := store.GetSession(context.Background(), "test-session-1")
	if err != nil {
		t.Fatalf("获取会话失败: %v", err)
	}
	if got == nil {
		t.Fatal("期望 GetSession 返回非 nil")
	}
	if got.SessionID != "test-session-1" {
		t.Errorf("SessionID 不匹配: want=test-session-1, got=%s", got.SessionID)
	}
	if got.EmployeeID != "EMP001" {
		t.Errorf("EmployeeID 不匹配: want=EMP001, got=%s", got.EmployeeID)
	}
	if got.MessageCount != 0 {
		t.Errorf("MessageCount 应为 0, got=%d", got.MessageCount)
	}
}

func TestSessionMeta_GetNonExistent(t *testing.T) {
	store, data := newTestStore(t)
	defer testutil.CleanDB(data)

	got, err := store.GetSession(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("查询不存在的会话应返回 nil, 而非错误: %v", err)
	}
	if got != nil {
		t.Errorf("查询不存在的会话应返回 nil, got=%+v", got)
	}
}

func TestSessionMeta_UpdateAndList(t *testing.T) {
	store, data := newTestStore(t)
	defer testutil.CleanDB(data)

	store.CreateSession(context.Background(), &infra.SessionMeta{
		SessionID: "s1", UserID: 1, EmployeeID: "E1",
		Status: infra.SessionStatusActive,
	})
	store.CreateSession(context.Background(), &infra.SessionMeta{
		SessionID: "s2", UserID: 1, EmployeeID: "E1",
		Status: infra.SessionStatusActive,
	})

	if err := store.UpdateSession(context.Background(), "s1", map[string]any{
		"status":  infra.SessionStatusCompleted,
		"summary": "已完成报销",
	}); err != nil {
		t.Fatalf("更新会话失败: %v", err)
	}

	got, _ := store.GetSession(context.Background(), "s1")
	if got.Status != infra.SessionStatusCompleted {
		t.Errorf("Status 应为 completed, got=%s", got.Status)
	}

	list, err := store.ListSessions(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("查询列表失败: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("期望 2 条会话, got=%d", len(list))
	}
}

func TestSessionMeta_DeleteSession(t *testing.T) {
	store, data := newTestStore(t)
	defer testutil.CleanDB(data)

	store.CreateSession(context.Background(), &infra.SessionMeta{
		SessionID: "to-delete", UserID: 1, Status: infra.SessionStatusActive,
	})

	if err := store.DeleteSession(context.Background(), "to-delete"); err != nil {
		t.Fatalf("删除会话失败: %v", err)
	}

	got, _ := store.GetSession(context.Background(), "to-delete")
	if got != nil {
		t.Errorf("删除后不应查询到会话, got=%+v", got)
	}
}

// ── Session Messages: 结构化存储 ──

func TestSaveAndGetMessages_StructuredFields(t *testing.T) {
	store, data := newTestStore(t)
	defer testutil.CleanDB(data)

	store.CreateSession(context.Background(), &infra.SessionMeta{
		SessionID: "msg-test", UserID: 1, Status: infra.SessionStatusActive,
	})

	msgs := []*schema.Message{
		schema.UserMessage("我要报销差旅费"),
		schema.AssistantMessage("好的，请上传票据图片。", nil),
	}

	if err := store.SaveMessages(context.Background(), "msg-test", msgs); err != nil {
		t.Fatalf("保存消息失败: %v", err)
	}

	history, err := store.GetHistory(context.Background(), "msg-test", 0)
	if err != nil {
		t.Fatalf("读取消息失败: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("期望 2 条消息, got=%d", len(history))
	}

	if history[0].Role != schema.User || history[0].Content != "我要报销差旅费" {
		t.Errorf("第一条应为 user 消息, got role=%s content=%s", history[0].Role, history[0].Content)
	}
	if history[1].Role != schema.Assistant {
		t.Errorf("第二条应为 assistant 消息, got role=%s", history[1].Role)
	}

	meta, _ := store.GetSession(context.Background(), "msg-test")
	if meta.MessageCount != 2 {
		t.Errorf("MessageCount 应为 2, got=%d", meta.MessageCount)
	}
}

func TestSaveAndGetMessages_ToolMessages(t *testing.T) {
	store, data := newTestStore(t)
	defer testutil.CleanDB(data)

	store.CreateSession(context.Background(), &infra.SessionMeta{
		SessionID: "tool-msg", UserID: 1, Status: infra.SessionStatusActive,
	})

	msgs := []*schema.Message{
		{ToolName: "recognize_invoice", Role: schema.Tool,
			Content: `{"amount":50000,"category":"差旅-住宿"}`},
	}

	if err := store.SaveMessages(context.Background(), "tool-msg", msgs); err != nil {
		t.Fatalf("保存工具消息失败: %v", err)
	}

	history, _ := store.GetHistory(context.Background(), "tool-msg", 0)
	if len(history) != 1 {
		t.Fatalf("期望 1 条消息, got=%d", len(history))
	}
	if history[0].ToolName != "recognize_invoice" {
		t.Errorf("ToolName 不匹配: want=recognize_invoice, got=%s", history[0].ToolName)
	}
}

func TestSaveAndGetMessages_WithLimit(t *testing.T) {
	store, data := newTestStore(t)
	defer testutil.CleanDB(data)

	store.CreateSession(context.Background(), &infra.SessionMeta{
		SessionID: "limit-test", UserID: 1, Status: infra.SessionStatusActive,
	})

	for i := 0; i < 10; i++ {
		store.SaveMessages(context.Background(), "limit-test",
			[]*schema.Message{schema.UserMessage("msg")})
	}

	history, _ := store.GetHistory(context.Background(), "limit-test", 3)
	if len(history) > 3 {
		t.Errorf("limit=3 最多返回 3 条, got=%d", len(history))
	}
}

func TestSaveAndGetMessages_RoundTrip(t *testing.T) {
	store, data := newTestStore(t)
	defer testutil.CleanDB(data)

	store.CreateSession(context.Background(), &infra.SessionMeta{
		SessionID: "roundtrip", UserID: 1, Status: infra.SessionStatusActive,
	})

	original := []*schema.Message{
		schema.UserMessage("你好"),
		schema.AssistantMessage("你好！我是 Reimbee。", nil),
		{
			Role:     schema.Tool,
			ToolName: "check_budget",
			Content:  `{"remaining": 100000, "usage_rate": 0.3}`,
		},
		schema.AssistantMessage("预算充足，可以提交。", nil),
	}

	if err := store.SaveMessages(context.Background(), "roundtrip", original); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	restored, _ := store.GetHistory(context.Background(), "roundtrip", 0)
	if len(restored) != 4 {
		t.Fatalf("往返后消息数不一致: want=4, got=%d", len(restored))
	}
	for i := range restored {
		if restored[i].Role != original[i].Role {
			t.Errorf("msg[%d] Role 不匹配: want=%s, got=%s", i, original[i].Role, restored[i].Role)
		}
		if restored[i].Content != original[i].Content {
			t.Errorf("msg[%d] Content 不匹配", i)
		}
		if original[i].ToolName != "" && restored[i].ToolName != original[i].ToolName {
			t.Errorf("msg[%d] ToolName 不匹配: want=%s, got=%s", i, original[i].ToolName, restored[i].ToolName)
		}
	}
}

func TestSaveAndGetMessages_SeqOrder(t *testing.T) {
	store, data := newTestStore(t)
	defer testutil.CleanDB(data)

	store.CreateSession(context.Background(), &infra.SessionMeta{
		SessionID: "seq-test", UserID: 1, Status: infra.SessionStatusActive,
	})

	store.SaveMessages(context.Background(), "seq-test",
		[]*schema.Message{schema.UserMessage("第一批")})
	store.SaveMessages(context.Background(), "seq-test",
		[]*schema.Message{schema.AssistantMessage("回复1", nil)})
	store.SaveMessages(context.Background(), "seq-test",
		[]*schema.Message{schema.UserMessage("第二批")})

	history, _ := store.GetHistory(context.Background(), "seq-test", 0)
	if len(history) != 3 {
		t.Fatalf("期望 3 条消息, got=%d", len(history))
	}

	expected := []string{"第一批", "回复1", "第二批"}
	for i, m := range history {
		if m.Content != expected[i] {
			t.Errorf("msg[%d] 顺序错误: want=%s, got=%s", i, expected[i], m.Content)
		}
	}
}

// ── Checkpoint State (v4 保留接口) ──

func TestCheckpointState_SaveAndGet(t *testing.T) {
	store, data := newTestStore(t)
	defer testutil.CleanDB(data)

	type testState struct{ Value string }
	state := &testState{Value: "hello"}

	if err := store.SaveCheckpointState(context.Background(), "s1", "reimbursement", state); err != nil {
		t.Fatalf("保存 Checkpoint 失败: %v", err)
	}

	var restored testState
	found, err := store.GetCheckpointState(context.Background(), "s1", "reimbursement", &restored)
	if err != nil {
		t.Fatalf("读取 Checkpoint 失败: %v", err)
	}
	if !found {
		t.Fatal("期望 found=true")
	}
	if restored.Value != "hello" {
		t.Errorf("状态值不匹配: want=hello, got=%s", restored.Value)
	}
}

func TestCheckpointState_CompatAlias(t *testing.T) {
	store, data := newTestStore(t)
	defer testutil.CleanDB(data)

	store.SaveState(context.Background(), "compat", "test", map[string]string{"k": "v"})

	var target map[string]string
	found, _ := store.GetState(context.Background(), "compat", "test", &target)
	if !found || target["k"] != "v" {
		t.Errorf("兼容别名 SaveState/GetState 失败: found=%v, val=%v", found, target)
	}
}
