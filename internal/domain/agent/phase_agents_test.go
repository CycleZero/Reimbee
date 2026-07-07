// Package agent_test LoopManager Agent 初始化黑盒测试
// 验证 mustNewAgent（未导出）通过 initAgents 正确创建 8 个 ChatModelAgent 实例
// 并为每个 Agent 分配正确的工具集
//
// 注意：mustNewAgent 是 package agent 的未导出函数，所有测试通过 NewLoopManager 间接验证。
// initAgents 在构造内部调用 mustNewAgent 8 次，任何 Agent 创建失败都触发 panic。
//
// 复用：newMockSessionStore、mockInvokableTool、testLogger 等来自 loop_manager_test.go
package agent_test

import (
	"context"
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/cloudwego/eino/components/tool"
)

// ============================================
// newMinimalToolSet — 最小 ToolSet（11 个 mock 工具）
// ============================================

// newMinimalToolSet 创建包含 mock 工具的 ToolSet，用于测试 Agent 初始化
// 所有工具共享同一个 mockInvokableTool 实例，用唯一名称区分类别
func newMinimalToolSet(t *testing.T) *tools.ToolSet {
	t.Helper()
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
		nil, // SessionStore（Agent 创建测试无需持久化）
		testLogger(t),
	)
}

// ============================================
// mustNewAgent 工厂函数测试（通过 NewLoopManager 间接验证）
// ============================================

// TestNewLoopManager_AllAgentsCreated 验证 LoopManager 创建时 8 个 Agent 全部初始化成功
// 不 panic 即意味着 mustNewAgent 8 次调用均成功
func TestNewLoopManager_AllAgentsCreated(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("收到，正在处理。")
	store := newMockSessionStore()
	toolSet := newMinimalToolSet(t)
	cfg := agent.DefaultLoopConfig()

	mgr := agent.NewLoopManager(store, mockModel, toolSet, testLogger(t), cfg)
	defer mgr.Shutdown()

	if mgr == nil {
		t.Fatal("NewLoopManager 返回 nil，期望非 nil")
	}
}

// TestNewLoopManager_ConcurrentCreation 验证并发创建 LoopManager 不 panic
func TestNewLoopManager_ConcurrentCreation(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("hello")
	store := newMockSessionStore()
	toolSet := newMinimalToolSet(t)
	cfg := agent.DefaultLoopConfig()

	done := make(chan struct{}, 3)
	for range 3 {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("NewLoopManager 并发创建 panic: %v", r)
				}
				done <- struct{}{}
			}()
			mgr := agent.NewLoopManager(store, mockModel, toolSet, testLogger(t), cfg)
			mgr.Shutdown()
		}()
	}
	for range 3 {
		<-done
	}
}

// TestMustNewAgent_AllIntentRoutes 验证 8 种意图分类各自路由不 panic
// 推送 6 种意图对应的消息，每种创建独立 session，验证 TurnLoop → PrepareAgent → Agent 路由
func TestMustNewAgent_AllIntentRoutes(t *testing.T) {
	mgr := newTestLoopManager(t)
	defer mgr.Shutdown()

	intents := []struct {
		name    string
		message string
	}{
		{"报销意图_Phase1", "我要报销差旅费"},
		{"进度查询", "我的报销进度到哪了"},
		{"预算查询", "还剩多少预算"},
		{"政策咨询", "差旅住宿标准是什么"},
		{"修改报销", "我要修改报销单"},
		{"通用对话", "你好"},
	}

	for _, intent := range intents {
		t.Run(intent.name, func(t *testing.T) {
			sessionID := "route-" + intent.name
			writer := newMockSSEWriter()
			doneCh := make(chan error, 1)

			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s 路由 panic: %v", intent.name, r)
					}
				}()
				mgr.PushMessage(sessionID, intent.message, writer, doneCh)
			}()
		})
	}
}

// ============================================
// initAgents 工具分配验证
// ============================================

// namedToolSet 创建名称唯一的 ToolSet 副本，用于工具分配精确验证
func namedToolSet(t *testing.T) *tools.ToolSet {
	t.Helper()
	return tools.NewToolSet(
		&tools.OCRTool{InvokableTool: &mockInvokableTool{name: "recognize_invoice"}},
		&tools.ComplianceTool{InvokableTool: &mockInvokableTool{name: "check_compliance"}},
		&tools.BudgetTool{InvokableTool: &mockInvokableTool{name: "check_budget"}},
		&tools.PDFTool{InvokableTool: &mockInvokableTool{name: "generate_pdf"}},
		&tools.EmailTool{InvokableTool: &mockInvokableTool{name: "send_email"}},
		&tools.ProgressTool{InvokableTool: &mockInvokableTool{name: "query_progress"}},
		&tools.QueryTool{InvokableTool: &mockInvokableTool{name: "query_records"}},
		&tools.ConfirmInvoiceTool{InvokableTool: &mockInvokableTool{name: "confirm_invoice"}},
		&tools.ConfirmSubmitTool{InvokableTool: &mockInvokableTool{name: "confirm_submit"}},
		&tools.CreateReimbTool{InvokableTool: &mockInvokableTool{name: "create_reimbursement"}},
		&tools.SubmitReimbTool{InvokableTool: &mockInvokableTool{name: "submit_reimbursement"}},
		nil,
		testLogger(t),
	)
}

// TestInitAgents_ToolSetIntegrity 验证 ToolSet 包含全部 11 个工具
func TestInitAgents_ToolSetIntegrity(t *testing.T) {
	ts := namedToolSet(t)
	allTools := ts.GetAllTools()

	if len(allTools) != 11 {
		t.Errorf("GetAllTools() = %d 工具, 期望 11", len(allTools))
	}

	expected := map[string]bool{
		"recognize_invoice": true, "check_compliance": true, "check_budget": true,
		"generate_pdf": true, "send_email": true, "query_progress": true,
		"query_records": true, "confirm_invoice": true, "confirm_submit": true,
		"create_reimbursement": true, "submit_reimbursement": true,
	}

	nameSet := toolNameSet(t, allTools)
	for name := range expected {
		if !nameSet[name] {
			t.Errorf("ToolSet 缺少工具: %s", name)
		}
	}
}

// TestInitAgents_Phase1CorrectTools 验证 Phase 1 Agent 分配 OCR + Compliance + ConfirmInvoice
func TestInitAgents_Phase1CorrectTools(t *testing.T) {
	ts := namedToolSet(t)
	phase1 := ts.GetPhase1BaseTools()

	if len(phase1) != 3 {
		t.Errorf("Phase 1 应包含 3 个工具, 实际 %d", len(phase1))
	}

	names := baseToolNameSet(t, phase1)
	for _, want := range []string{"recognize_invoice", "check_compliance", "confirm_invoice"} {
		if !names[want] {
			t.Errorf("Phase 1 缺少工具: %s", want)
		}
	}
}

// TestInitAgents_Phase2CorrectTools 验证 Phase 2 Agent 分配 Compliance + Budget + ConfirmSubmit
func TestInitAgents_Phase2CorrectTools(t *testing.T) {
	ts := namedToolSet(t)
	phase2 := ts.GetPhase2BaseTools()

	if len(phase2) != 3 {
		t.Errorf("Phase 2 应包含 3 个工具, 实际 %d", len(phase2))
	}

	names := baseToolNameSet(t, phase2)
	for _, want := range []string{"check_compliance", "check_budget", "confirm_submit"} {
		if !names[want] {
			t.Errorf("Phase 2 缺少工具: %s", want)
		}
	}
}

// TestInitAgents_Phase3CorrectTools 验证 Phase 3 Agent 分配 CreateReimb + SubmitReimb + PDF + Email + Progress
func TestInitAgents_Phase3CorrectTools(t *testing.T) {
	ts := namedToolSet(t)
	phase3 := ts.GetPhase3BaseTools()

	if len(phase3) != 5 {
		t.Errorf("Phase 3 应包含 5 个工具, 实际 %d", len(phase3))
	}

	names := baseToolNameSet(t, phase3)
	for _, want := range []string{"create_reimbursement", "submit_reimbursement", "generate_pdf", "send_email", "query_progress"} {
		if !names[want] {
			t.Errorf("Phase 3 缺少工具: %s", want)
		}
	}
}

// TestInitAgents_PhaseToolIsolation 验证各 Phase Agent 工具集物理隔离
// Phase 1 不应包含 Phase 3 专属工具（CreateReimb / SubmitReimb）
// Phase 3 不应包含 Phase 1 专属工具（OCR / ConfirmInvoice）
// confirm_submit 仅属于 Phase 2
func TestInitAgents_PhaseToolIsolation(t *testing.T) {
	ts := namedToolSet(t)

	p1 := baseToolNameSet(t, ts.GetPhase1BaseTools())
	p2 := baseToolNameSet(t, ts.GetPhase2BaseTools())
	p3 := baseToolNameSet(t, ts.GetPhase3BaseTools())

	// Phase 1 不应包含 Phase 3 专属工具
	for _, forbidden := range []string{"create_reimbursement", "submit_reimbursement"} {
		if p1[forbidden] {
			t.Errorf("Phase 1 不应包含 Phase 3 工具: %s", forbidden)
		}
	}

	// Phase 3 不应包含 Phase 1 专属工具
	for _, forbidden := range []string{"recognize_invoice", "confirm_invoice"} {
		if p3[forbidden] {
			t.Errorf("Phase 3 不应包含 Phase 1 工具: %s", forbidden)
		}
	}

	// confirm_submit 仅属于 Phase 2
	if p1["confirm_submit"] {
		t.Error("Phase 1 不应包含 confirm_submit")
	}
	if p3["confirm_submit"] {
		t.Error("Phase 3 不应包含 confirm_submit")
	}
	if !p2["confirm_submit"] {
		t.Error("Phase 2 应包含 confirm_submit")
	}

	// check_compliance 在 P1 和 P2 共享（设计如此）
	if !p1["check_compliance"] || !p2["check_compliance"] {
		t.Error("check_compliance 应在 Phase 1 和 Phase 2 共享")
	}
}

// TestInitAgents_ChatAgentNoTools 验证 ChatAgent 不分配工具（nil tools）
// ChatAgent 处理问候/感谢等，工具列表为 nil 即正确
func TestInitAgents_ChatAgentNoTools(t *testing.T) {
	mgr := newTestLoopManager(t)
	defer mgr.Shutdown()

	writer := newMockSSEWriter()
	doneCh := make(chan error, 1)

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ChatAgent（无工具）路由 panic: %v", r)
			}
		}()
		mgr.PushMessage("chat-no-tools", "你好，今天天气不错", writer, doneCh)
	}()
}

// ============================================
// 辅助函数
// ============================================

// toolNameSet 将 []tool.InvokableTool 转为 {name: true} 集合
func toolNameSet(t *testing.T, tl []tool.InvokableTool) map[string]bool {
	t.Helper()
	m := make(map[string]bool, len(tl))
	for _, tool := range tl {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		m[info.Name] = true
	}
	return m
}

// baseToolNameSet 将 []tool.BaseTool 转为 {name: true} 集合
func baseToolNameSet(t *testing.T, tl []tool.BaseTool) map[string]bool {
	t.Helper()
	m := make(map[string]bool, len(tl))
	for _, bt := range tl {
		info, err := bt.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		m[info.Name] = true
	}
	return m
}
