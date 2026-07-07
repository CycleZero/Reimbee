package tools_test

import (
	"context"
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ============================================
// Mock 实现
// ============================================

// mockInvokableTool 实现 tool.InvokableTool 接口，用于测试 ToolSet
type mockInvokableTool struct {
	name string
}

func (m *mockInvokableTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: m.name}, nil
}

func (m *mockInvokableTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	return `{"status":"ok"}`, nil
}

// testLogger 创建静默日志器
func testLogger(t *testing.T) *log.Logger {
	t.Helper()
	return &log.Logger{Logger: zap.NewNop()}
}

// newMockToolSet 创建带有 mock 工具的 ToolSet，各工具名称不同以区分类别
func newMockToolSet(t *testing.T) *tools.ToolSet {
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
		nil, // v3.0: SessionStore（测试中为 nil）
		testLogger(t),
	)
}

// ============================================
// GetPhase1Tools 测试
// ============================================

// TestToolSet_GetPhase1Tools 验证 Phase 1（信息收集）返回 2 个工具：OCR + Compliance
func TestToolSet_GetPhase1Tools(t *testing.T) {
	ts := newMockToolSet(t)

	got := ts.GetPhase1Tools()

	if len(got) != 3 {
		t.Errorf("GetPhase1Tools() 应返回 3 个工具，实际返回 %d 个", len(got))
	}

	// 验证工具名称
	names := make(map[string]bool)
	for _, tool := range got {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		names[info.Name] = true
	}

	if !names["recognize_invoice"] {
		t.Error("Phase 1 工具应包含 recognize_invoice (OCR)")
	}
	if !names["check_compliance"] {
		t.Error("Phase 1 工具应包含 check_compliance (Compliance)")
	}
	if !names["confirm_invoice"] {
		t.Error("Phase 1 工具应包含 confirm_invoice (ConfirmInvoice)")
	}
}

// ============================================
// GetPhase2Tools 测试
// ============================================

// TestToolSet_GetPhase2Tools 验证 Phase 2（校验确认）返回 3 个工具：Compliance + Budget + ConfirmSubmit
func TestToolSet_GetPhase2Tools(t *testing.T) {
	ts := newMockToolSet(t)

	got := ts.GetPhase2Tools()

	if len(got) != 3 {
		t.Errorf("GetPhase2Tools() 应返回 3 个工具，实际返回 %d 个", len(got))
	}

	names := make(map[string]bool)
	for _, tool := range got {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		names[info.Name] = true
	}

	if !names["check_compliance"] {
		t.Error("Phase 2 工具应包含 check_compliance (Compliance)")
	}
	if !names["check_budget"] {
		t.Error("Phase 2 工具应包含 check_budget (Budget)")
	}
}

// ============================================
// GetPhase3Tools 测试
// ============================================

// TestToolSet_GetPhase3Tools 验证 Phase 3（执行提交）返回 5 个工具
func TestToolSet_GetPhase3Tools(t *testing.T) {
	ts := newMockToolSet(t)

	got := ts.GetPhase3Tools()

	if len(got) != 5 {
		t.Errorf("GetPhase3Tools() 应返回 5 个工具，实际返回 %d 个", len(got))
	}

	names := make(map[string]bool)
	for _, tool := range got {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		names[info.Name] = true
	}

	if !names["generate_pdf"] {
		t.Error("Phase 3 工具应包含 generate_pdf (PDF)")
	}
	if !names["send_email"] {
		t.Error("Phase 3 工具应包含 send_email (Email)")
	}
	if !names["query_progress"] {
		t.Error("Phase 3 工具应包含 query_progress (Progress)")
	}
}

// ============================================
// GetAllTools 测试
// ============================================

// TestToolSet_GetAllTools 验证 GetAllTools 返回全部 7 个工具
func TestToolSet_GetAllTools(t *testing.T) {
	ts := newMockToolSet(t)

	got := ts.GetAllTools()

	if len(got) != 11 {
		t.Errorf("GetAllTools() 应返回 11 个工具，实际返回 %d 个", len(got))
	}

	names := make(map[string]bool)
	for _, tool := range got {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		names[info.Name] = true
	}

	expectedNames := []string{
		"recognize_invoice",
		"check_compliance",
		"check_budget",
		"generate_pdf",
		"send_email",
		"query_progress",
		"query_records",
	}

	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("GetAllTools 应包含 %s", name)
		}
	}
}

// ============================================
// 边界测试
// ============================================

// TestToolSet_PhaseSeparation 验证各 Phase 返回的工具不重叠（除 Compliance 外）
func TestToolSet_PhaseSeparation(t *testing.T) {
	ts := newMockToolSet(t)

	phase1 := ts.GetPhase1Tools()
	phase2 := ts.GetPhase2Tools()
	phase3 := ts.GetPhase3Tools()

	// Phase 1 不应包含 Phase 3 的工具
	phase1Names := toolNames(t, phase1)
	phase3Names := toolNames(t, phase3)

	for _, n1 := range phase1Names {
		if n1 == "check_compliance" {
			continue // Compliance 在 Phase 1 和 Phase 2 共享
		}
		for _, n3 := range phase3Names {
			if n1 == n3 {
				t.Errorf("Phase 1 和 Phase 3 不应共享工具 %s", n1)
			}
		}
	}

	// Phase 2 不应包含 Phase 3 的工具（除 Compliance 外，它在 P1/P2 共享）
	phase2Names := toolNames(t, phase2)
	for _, n2 := range phase2Names {
		if n2 == "check_compliance" {
			continue
		}
		for _, n3 := range phase3Names {
			if n2 == n3 {
				t.Errorf("Phase 2 和 Phase 3 不应共享工具 %s", n2)
			}
		}
	}
}

// toolNames 提取工具列表中各工具的名称
func toolNames(t *testing.T, tools []tool.InvokableTool) []string {
	t.Helper()
	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		info, err := tl.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		names = append(names, info.Name)
	}
	return names
}

// TestToolSet_AllToolsIncludesAllPhases 验证 GetAllTools 是各 Phase 工具的超集
func TestToolSet_AllToolsIncludesAllPhases(t *testing.T) {
	ts := newMockToolSet(t)

	all := ts.GetAllTools()
	phase1 := ts.GetPhase1Tools()
	phase2 := ts.GetPhase2Tools()
	phase3 := ts.GetPhase3Tools()

	allNames := toolNames(t, all)

	// Phase 1 的所有工具应在 All 中
	for _, name := range toolNames(t, phase1) {
		if !contains(allNames, name) {
			t.Errorf("GetAllTools 缺少 Phase 1 工具: %s", name)
		}
	}
	// Phase 2 的所有工具应在 All 中
	for _, name := range toolNames(t, phase2) {
		if !contains(allNames, name) {
			t.Errorf("GetAllTools 缺少 Phase 2 工具: %s", name)
		}
	}
	// Phase 3 的所有工具应在 All 中
	for _, name := range toolNames(t, phase3) {
		if !contains(allNames, name) {
			t.Errorf("GetAllTools 缺少 Phase 3 工具: %s", name)
		}
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
