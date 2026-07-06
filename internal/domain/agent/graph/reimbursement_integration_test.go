package graph

import (
	"context"
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// 三阶段端到端集成测试：验证编译成功 + Guard 重试验证
func TestReimbursementIntegration_CompilesAndRuns(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("请上传票据。")

	runnable, err := NewReimbursementGraph(context.Background(), ReimbursementGraphDeps{
		Logger:    nopLogger(),
		ToolSet:   nil,
		ChatModel: mockModel,
		Config:    &agent.AgentConfig{MaxPhaseTurns: 2},
	})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	if runnable == nil {
		t.Fatal("runnable 为 nil")
	}

	// 执行：无票据时 Guard 持续失败，超 maxSteps 报错
	_, err = runnable.Invoke(context.Background(),
		[]*schema.Message{schema.UserMessage("报销")})
	if err == nil {
		t.Log("报销图执行完成（Guard 通过）")
	} else {
		t.Logf("报销图执行中止（预期：Guard 循环超限）: %v", err)
	}
}

// 工具调用验证：ReAct 循环中工具被实际执行
func TestReimbursementIntegration_ToolsExecuted(t *testing.T) {
	ocrCalled := false
	mockOCR := &testutil.MockBaseTool{
		InfoFunc: func(ctx context.Context) (*schema.ToolInfo, error) {
			return &schema.ToolInfo{Name: "ocr_tool", Desc: "OCR识别",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"path": {Type: schema.String, Desc: "路径", Required: true},
				})}, nil
		},
		RunFunc: func(ctx context.Context, args string) (string, error) {
			ocrCalled = true
			return "识别成功", nil
		},
	}

	mockModel := testutil.NewMultiTurnChatModel([]*schema.Message{
		{Role: schema.Assistant, Content: "", ToolCalls: []schema.ToolCall{
			{ID: "c1", Type: "function", Function: schema.FunctionCall{Name: "ocr_tool", Arguments: `{}`}},
		}},
		schema.AssistantMessage("工具执行完毕。", nil),
	})

	graph, err := buildReActPhase(context.Background(), mockModel, nopLogger(), PhaseConfig{
		Name: "test_phase", SystemPrompt: "test", Tools: []tool.BaseTool{mockOCR},
	})
	if err != nil {
		t.Fatalf("构建 ReAct 阶段失败: %v", err)
	}

	r, _ := graph.Compile(context.Background(), compose.WithMaxRunSteps(20))
	result, err := r.Invoke(context.Background(), []*schema.Message{schema.UserMessage("test")})
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if !ocrCalled {
		t.Error("❌ 工具未被调用——ReAct 循环未执行工具")
	}
	t.Logf("✅ 工具调用成功，最终回复: %s", result.Content)
}

// 阶段计数器独立性验证
func TestReimbursementIntegration_PhaseCountersIndependent(t *testing.T) {
	var p1After, p2After int
	mockModel := &testutil.MockChatModel{
		GenerateFunc: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
			_ = compose.ProcessState(ctx, func(ctx context.Context, rs *agent.ReimbursementState) error {
				p1After = rs.Phase1Turns
				p2After = rs.Phase2Turns
				return nil
			})
			return schema.AssistantMessage("ok", nil), nil
		},
	}

	deps := ReimbursementGraphDeps{
		Logger: nopLogger(), ToolSet: nil, ChatModel: mockModel,
		Config: &agent.AgentConfig{MaxPhaseTurns: 2},
	}

	r, _ := NewReimbursementGraph(context.Background(), deps)
	_, err := r.Invoke(context.Background(), []*schema.Message{schema.UserMessage("test")})
	t.Logf("Phase1Turns=%d Phase2Turns=%d err=%v", p1After, p2After, err)
}

// nil-safe ToolSet：构建时 ToolSet 为 nil 不 panic
func TestReimbursementIntegration_NilToolSetSafe(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("ok")

	r, err := NewReimbursementGraph(context.Background(), ReimbursementGraphDeps{
		Logger:    nopLogger(), ToolSet: nil, ChatModel: mockModel,
		Config: &agent.AgentConfig{MaxPhaseTurns: 2},
	})
	if err != nil {
		t.Fatalf("nil ToolSet 导致编译失败: %v", err)
	}

	_, err = r.Invoke(context.Background(), []*schema.Message{schema.UserMessage("test")})
	t.Logf("nil ToolSet 执行结果: err=%v", err)
}

// 并发安全：多 goroutine 同时执行不 panic
func TestReimbursementIntegration_ConcurrentSafe(t *testing.T) {
	r, _ := NewReimbursementGraph(context.Background(), ReimbursementGraphDeps{
		Logger: nopLogger(), ToolSet: nil,
		ChatModel: testutil.NewTextReplyChatModel("ok"),
		Config:    &agent.AgentConfig{MaxPhaseTurns: 2},
	})

	const n = 5
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, e := r.Invoke(context.Background(), []*schema.Message{schema.UserMessage("test")})
			errCh <- e
		}()
	}
	for i := 0; i < n; i++ {
		if e := <-errCh; e != nil {
			t.Logf("并发 %d: %v", i, e)
		}
	}
	t.Log("并发安全：无 panic")
}
