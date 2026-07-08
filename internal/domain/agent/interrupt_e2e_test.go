// Package agent 中断流程端到端测试
//
// 测试完整的 interrupt → approve → resume → done 链路。
// 使用脚本化 fakeModel 替代真实 LLM，通过 httptest 捕获 SSE 事件流。
package agent

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/common"
	agenttools "github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades"
	"github.com/gin-gonic/gin"
)

// fakeStreamingModel 实现 blades.ModelProvider，按顺序返回预设的 Message 序列。
type fakeStreamingModel struct {
	responses []*blades.Message
	callCount int
}

func (m *fakeStreamingModel) Name() string { return "fake-e2e" }

func (m *fakeStreamingModel) Generate(_ context.Context, _ *blades.ModelRequest) (*blades.ModelResponse, error) {
	idx := m.callCount
	m.callCount++
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	return &blades.ModelResponse{Message: m.responses[idx]}, nil
}

func (m *fakeStreamingModel) NewStreaming(ctx context.Context, req *blades.ModelRequest) blades.Generator[*blades.ModelResponse, error] {
	return func(yield func(*blades.ModelResponse, error) bool) {
		resp, err := m.Generate(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		yield(resp, nil)
	}
}

// newAssistantMsg 创建状态为 completed 的助手消息。
// 注意：blades.AssistantMessage() 不设置 Status，必须用 NewAssistantMessage 构造。
func newAssistantMsg(text string) *blades.Message {
	msg := blades.NewAssistantMessage(blades.StatusCompleted)
	msg.Parts = []blades.Part{blades.TextPart{Text: text}}
	return msg
}

// TestInterruptFlow_E2E 测试完整的中断→审批→恢复流程
func TestInterruptFlow_E2E(t *testing.T) {
	data := testutil.NewTestData()
	logger := log.GetLogger()

	fakeModel := &fakeStreamingModel{
		responses: []*blades.Message{
			// Turn 1: LLM 调用 test_interrupt → 触发中断
			{
				Role:   blades.RoleTool,
				Status: blades.StatusCompleted,
				Parts: []blades.Part{
					blades.ToolPart{ID: "call_001", Name: "test_interrupt", Request: "{}"},
				},
			},
			// Turn 2: 审批通过后恢复，LLM 返回确认
			newAssistantMsg("中断测试完成，操作已成功执行！"),
		},
	}

	testTool := agenttools.NewTestInterruptTool()
	toolSet := &agenttools.ToolSet{TestInterrupt: testTool}
	sessionRepo := infra.NewSessionRepo(data.DB, nil, logger)
	ag := NewReimburseAgent(fakeModel, toolSet, sessionRepo, DefaultConfig(), logger)

	// ===== Step 1: 首次 Run，触发中断 =====
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest("GET", "/?session_id=e2e-interrupt&message=执行中断测试", nil)
	ctx1 := common.SetRequestMetadata(c1.Request.Context(), &common.RequestMetadata{
		Role: "employee", EmployeeID: "EMP001", UserID: 1,
	})
	gw1, _ := NewGinSSEWriter(c1)

	err := ag.Run(ctx1, RunParams{
		SessionID: "e2e-interrupt", Message: "请执行测试中断",
		Role: "employee", EmployeeID: "EMP001", EmployeeName: "测试员工", UserID: 1,
	}, gw1)
	if err != nil {
		t.Fatalf("首次 Run 返回错误: %v", err)
	}

	body1 := w1.Body.String()
	t.Logf("SSE 首次响应:\n%s", body1)

	if !strings.Contains(body1, "interrupted") {
		t.Error("应发送 interrupted 事件")
	}
	if !strings.Contains(body1, "test_interrupt") {
		t.Error("interrupted 事件应包含工具名")
	}

	// ===== Step 2: 审批通过，恢复执行 =====
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/", nil)
	ctx2 := common.SetRequestMetadata(c2.Request.Context(), &common.RequestMetadata{
		Role: "employee", EmployeeID: "EMP001", UserID: 1,
	})
	gw2, _ := NewGinSSEWriter(c2)

	err = ag.HandleApprove(ctx2, "e2e-interrupt", true, "确认执行", gw2)
	if err != nil {
		t.Fatalf("HandleApprove 返回错误: %v", err)
	}

	body2 := w2.Body.String()
	t.Logf("SSE 恢复响应:\n%s", body2)

	if !strings.Contains(body2, "done") {
		t.Error("恢复后应发送 done 事件")
	}
	if !strings.Contains(body2, "中断测试完成") {
		t.Error("恢复后 LLM 应返回确认消息")
	}
}

// TestInterruptFlow_Reject 测试审批拒绝场景
func TestInterruptFlow_Reject(t *testing.T) {
	data := testutil.NewTestData()
	logger := log.GetLogger()

	fakeModel := &fakeStreamingModel{
		responses: []*blades.Message{
			{
				Role:   blades.RoleTool,
				Status: blades.StatusCompleted,
				Parts: []blades.Part{
					blades.ToolPart{ID: "call_001", Name: "test_interrupt", Request: "{}"},
				},
			},
			newAssistantMsg("不会到达"),
		},
	}

	testTool := agenttools.NewTestInterruptTool()
	toolSet := &agenttools.ToolSet{TestInterrupt: testTool}
	sessionRepo := infra.NewSessionRepo(data.DB, nil, logger)
	ag := NewReimburseAgent(fakeModel, toolSet, sessionRepo, DefaultConfig(), logger)

	// Step 1: 触发中断
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest("GET", "/?session_id=e2e-reject&message=test", nil)
	ctx1 := common.SetRequestMetadata(c1.Request.Context(), &common.RequestMetadata{
		Role: "employee", EmployeeID: "EMP001", UserID: 1,
	})
	gw1, _ := NewGinSSEWriter(c1)
	ag.Run(ctx1, RunParams{
		SessionID: "e2e-reject", Message: "test", Role: "employee",
		EmployeeID: "EMP001", EmployeeName: "测试", UserID: 1,
	}, gw1)

	// Step 2: 拒绝审批
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/", nil)
	ctx2 := common.SetRequestMetadata(c2.Request.Context(), &common.RequestMetadata{
		Role: "employee", EmployeeID: "EMP001", UserID: 1,
	})
	gw2, _ := NewGinSSEWriter(c2)

	err := ag.HandleApprove(ctx2, "e2e-reject", false, "不符合要求，拒绝", gw2)
	if err != nil {
		t.Fatalf("HandleApprove(拒绝) 返回错误: %v", err)
	}

	body2 := w2.Body.String()
	t.Logf("SSE 拒绝响应:\n%s", body2)

	// 拒绝后 Agent 继续执行（LLM 看到 rejected 结果后回复），应正常完成
	if !strings.Contains(body2, "done") {
		t.Error("拒绝审批后应正常结束（done 事件）")
	}
	// 验证 session 中工具结果包含 rejected 状态
	session, _ := GetOrCreate(c2.Request.Context(), "e2e-reject", sessionRepo)
	msgs, _ := session.History(c2.Request.Context())
	found := false
	for _, m := range msgs {
		for _, p := range m.Parts {
			if tp, ok := any(p).(blades.ToolPart); ok && tp.Completed {
				if strings.Contains(tp.Response, "rejected") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("session 中应包含 rejected 状态的工具结果")
	}
}

// TestInterruptFlow_WithoutPendingTool 测试无待审批操作时的错误处理
func TestInterruptFlow_WithoutPendingTool(t *testing.T) {
	data := testutil.NewTestData()
	logger := log.GetLogger()

	fakeModel := &fakeStreamingModel{
		responses: []*blades.Message{
			newAssistantMsg("无需中断的回复"),
		},
	}

	testTool := agenttools.NewTestInterruptTool()
	toolSet := &agenttools.ToolSet{TestInterrupt: testTool}
	sessionRepo := infra.NewSessionRepo(data.DB, nil, logger)
	ag := NewReimburseAgent(fakeModel, toolSet, sessionRepo, DefaultConfig(), logger)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?session_id=e2e-no-pending&message=hi", nil)
	ctx := common.SetRequestMetadata(c.Request.Context(), &common.RequestMetadata{
		Role: "employee", EmployeeID: "EMP001", UserID: 1,
	})
	gw, _ := NewGinSSEWriter(c)
	ag.Run(ctx, RunParams{SessionID: "e2e-no-pending", Message: "hi", Role: "employee", EmployeeID: "EMP001", EmployeeName: "测试", UserID: 1}, gw)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/", nil)
	gw2, _ := NewGinSSEWriter(c2)

	err := ag.HandleApprove(c2.Request.Context(), "e2e-no-pending", true, "", gw2)
	if err == nil {
		t.Error("无待审批操作时 HandleApprove 应返回错误")
	}
	t.Logf("预期的错误: %v", err)

	if !strings.Contains(w2.Body.String(), "没有待审批的操作") {
		t.Error("应包含错误信息")
	}
}
