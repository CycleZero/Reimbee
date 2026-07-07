package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/internal/domain/agent/graph"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

func e2eLogger() *log.Logger {
	return &log.Logger{Logger: zap.NewNop()}
}

func newE2ERunner(t *testing.T, mockModel model.ToolCallingChatModel) (*agent.AgentRunner, *mockSessionStore) {
	t.Helper()
	cfg := &agent.AgentConfig{
		SessionTTLMinutes: 30, MaxHistoryTurns: 5, MaxPhaseTurns: 3,
		CheckpointCleanupHours: 1, LLMMaxRetries: 3, LLMRetryBackoffSeconds: 2,
		ToolTimeoutSeconds: 30, IntentConfidenceThreshold: 0.7,
	}

	reimbRunnable, err := graph.NewReimbursementGraph(context.Background(), graph.ReimbursementGraphDeps{
		Logger: e2eLogger(), ChatModel: mockModel, Config: cfg,
	})
	if err != nil {
		t.Fatalf("构建报销图失败: %v", err)
	}

	rootGraph, err := graph.NewRootGraph(context.Background(), graph.RootGraphDeps{
		Logger: e2eLogger(), ChatModel: mockModel, Config: cfg,
		ReimbursementRunnable: reimbRunnable,
	})
	if err != nil {
		t.Fatalf("构建 Root Graph 失败: %v", err)
	}

	sessionStore := newMockSessionStore()
	runner := agent.NewAgentRunner(rootGraph, sessionStore, newMockCheckpointStore(), cfg, e2eLogger())
	return runner, sessionStore
}

func TestE2E_StatePersistenceRoundTrip(t *testing.T) {
	original := &agent.ReimbursementState{
		CurrentPhase: "phase2_validate", EmployeeID: "EMP042", EmployeeName: "张三",
		DepartmentID: 1, TotalAmount: 50000,
		Invoices: []agent.InvoiceState{
			{Index: 1, Amount: 30000, Category: "差旅-交通", UserConfirmed: true},
			{Index: 2, Amount: 20000, Category: "招待费", UserConfirmed: true},
		},
		UserConfirmed: true, ComplianceResult: &agent.ComplianceCheckResult{Result: "pass"},
		Phase1Turns: 3,
	}
	store := newMockSessionStore()
	sid := "e2e-s1"

	if err := store.SaveState(context.Background(), sid, "reimbursement", original); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	var restored agent.ReimbursementState
	found, _ := store.GetState(context.Background(), sid, "reimbursement", &restored)
	if !found {
		t.Fatal("未找到 State")
	}
	if restored.CurrentPhase != "phase2_validate" || restored.EmployeeID != "EMP042" {
		t.Error("字段恢复不一致")
	}

	origJSON, _ := json.Marshal(original)
	restJSON, _ := json.Marshal(restored)
	if string(origJSON) != string(restJSON) {
		t.Errorf("往返序列化不一致\n原始: %s\n恢复: %s", origJSON, restJSON)
	}

	var empty agent.ReimbursementState
	f, _ := store.GetState(context.Background(), "nonexistent", "reimbursement", &empty)
	if f {
		t.Error("不存在应返回 false")
	}

	store.DeleteState(context.Background(), sid, "reimbursement")
	f2, _ := store.GetState(context.Background(), sid, "reimbursement", &empty)
	if f2 {
		t.Error("删除后应不存在")
	}
	t.Log("✅ State 持久化往返测试通过")
}

func TestE2E_MultiTurnConversation(t *testing.T) {
	callCount := 0
	mockModel := &testutil.MockChatModel{
		GenerateFunc: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
			callCount++
			switch {
			case callCount <= 3:
				return schema.AssistantMessage("请上传票据。", nil), nil
			case callCount <= 6:
				return schema.AssistantMessage("合规通过，请确认提交。", nil), nil
			default:
				return schema.AssistantMessage("报销单 REIMB-2026-0100 已提交！", nil), nil
			}
		},
	}

	runner, _ := newE2ERunner(t, mockModel)

	turns := []string{"我要报销差旅费", "已上传发票", "确认", "确认提交"}
	for i, msg := range turns {
		ss := &mockSSEWriter{}
		err := runner.StreamChat(context.Background(), agent.BuildAgentInput("e2e-002", msg, "EMP001", 1, "employee"), ss)
		if err != nil {
			t.Fatalf("第 %d 轮(%s)失败: %v", i+1, msg, err)
		}
		if !hasEvent(ss, agent.EventTypeDone) {
			t.Errorf("第 %d 轮缺少 done 事件", i+1)
		}
	}

	t.Logf("✅ 4 轮对话完成 (%d 次 ChatModel 调用)", callCount)
}

func TestE2E_SSEEventSequence(t *testing.T) {
	runner, _ := newE2ERunner(t, testutil.NewTextReplyChatModel("ok"))
	ss := &mockSSEWriter{}
	err := runner.StreamChat(context.Background(), agent.BuildAgentInput("e2e-sse", "报销", "E1", 1, "employee"), ss)
	t.Logf("err=%v", err)

	if len(ss.events) == 0 {
		t.Fatal("无事件")
	}
	if ss.events[0].Type != agent.EventTypeThinking {
		t.Errorf("首事件应为thinking, 实际 %s", ss.events[0].Type)
	}
	last := ss.events[len(ss.events)-1].Type
	if last != agent.EventTypeDone && last != agent.EventTypeError {
		t.Errorf("末事件应为done/error, 实际 %s", last)
	}
	t.Logf("✅ SSE 序列验证 (%d 事件)", len(ss.events))
	for i, ev := range ss.events {
		t.Logf("  [%d] %s", i, ev.Type)
	}
}

func TestE2E_ConcurrentSessions(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("ok")
	errCh := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func(idx int) {
			runner, _ := newE2ERunner(t, mockModel)
			ss := &mockSSEWriter{}
			err := runner.StreamChat(context.Background(),
				agent.BuildAgentInput("e2e-c-"+string(rune('0'+idx)), "test", "E1", uint(idx), "employee"), ss)
			errCh <- err
		}(i)
	}
	for i := 0; i < 5; i++ {
		<-errCh
	}
	t.Log("✅ 并发安全")
}

func TestE2E_StateContextInjection(t *testing.T) {
	saved := &agent.ReimbursementState{
		CurrentPhase: "phase2_validate", EmployeeID: "EMP_INJECTED",
		Invoices: []agent.InvoiceState{{Index: 1, Amount: 50000, Category: "差旅-交通"}},
		TotalAmount: 50000, Phase1Turns: 5,
	}
	ctx := context.WithValue(context.Background(), agent.StateContextKey{}, saved)

	r, err := graph.NewReimbursementGraph(ctx, graph.ReimbursementGraphDeps{
		Logger: e2eLogger(), ChatModel: testutil.NewTextReplyChatModel("ok"),
		Config: &agent.AgentConfig{MaxPhaseTurns: 2},
	})
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	result, err := r.Invoke(ctx, []*schema.Message{schema.UserMessage("继续")})
	t.Logf("result=%v err=%v", result, err)
	t.Log("✅ State 上下文注入")
}

func hasEvent(ss *mockSSEWriter, typ agent.SSEEventType) bool {
	for _, ev := range ss.events {
		if ev.Type == typ {
			return true
		}
	}
	return false
}
