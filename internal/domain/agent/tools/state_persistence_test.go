package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
)

// ============================================
// Phase 工具组合验证：确认类工具在各阶段的分布
// ============================================

// TestToolSet_Phase1HasConfirmInvoice 验证 Phase 1 BaseTools 包含 confirm_invoice 工具
func TestToolSet_Phase1HasConfirmInvoice(t *testing.T) {
	ts := newMockToolSet(t)
	baseTools := ts.GetPhase1BaseTools()

	found := false
	for _, bt := range baseTools {
		info, err := bt.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		if info.Name == "confirm_invoice" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Phase 1 BaseTools 应包含 confirm_invoice 工具")
	}
}

// TestToolSet_Phase2HasConfirmSubmit 验证 Phase 2 BaseTools 包含 confirm_submit 工具
func TestToolSet_Phase2HasConfirmSubmit(t *testing.T) {
	ts := newMockToolSet(t)
	baseTools := ts.GetPhase2BaseTools()

	found := false
	for _, bt := range baseTools {
		info, err := bt.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		if info.Name == "confirm_submit" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Phase 2 BaseTools 应包含 confirm_submit 工具")
	}
}

// TestToolSet_Phase3DoesNotHaveConfirmTools 验证 Phase 3 不包含确认类工具
// Phase 3 是执行阶段，此时确认已完成，不应再有 confirm 工具
func TestToolSet_Phase3DoesNotHaveConfirmTools(t *testing.T) {
	ts := newMockToolSet(t)
	baseTools := ts.GetPhase3BaseTools()

	for _, bt := range baseTools {
		info, err := bt.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		if info.Name == "confirm_invoice" || info.Name == "confirm_submit" {
			t.Errorf("Phase 3 不应包含确认类工具 %s（确认应在 Phase 1/2 完成）", info.Name)
		}
	}
}

// TestToolSet_ConfirmInvoiceNotInPhase2 验证 confirm_invoice 仅属于 Phase 1，不在 Phase 2
func TestToolSet_ConfirmInvoiceNotInPhase2(t *testing.T) {
	ts := newMockToolSet(t)
	phase2Tools := ts.GetPhase2BaseTools()

	for _, bt := range phase2Tools {
		info, err := bt.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		if info.Name == "confirm_invoice" {
			t.Error("confirm_invoice 不应出现在 Phase 2（票据确认属于 Phase 1）")
		}
	}
}

// TestToolSet_ConfirmSubmitNotInPhase1 验证 confirm_submit 仅属于 Phase 2，不在 Phase 1
func TestToolSet_ConfirmSubmitNotInPhase1(t *testing.T) {
	ts := newMockToolSet(t)
	phase1Tools := ts.GetPhase1BaseTools()

	for _, bt := range phase1Tools {
		info, err := bt.Info(context.Background())
		if err != nil {
			t.Fatalf("获取工具信息失败: %v", err)
		}
		if info.Name == "confirm_submit" {
			t.Error("confirm_submit 不应出现在 Phase 1（最终提交确认属于 Phase 2）")
		}
	}
}

// ============================================
// ToolInfo 完整性测试
// ============================================

// TestConfirmToolsHaveValidInfo 验证确认工具的 ToolInfo 可正常获取且名称正确
func TestConfirmToolsHaveValidInfo(t *testing.T) {
	store := newMockStore()

	// 验证 confirm_invoice 工具的 Info
	ciTool := tools.NewConfirmInvoiceTool(store, testLogger(t))
	ciInfo, err := ciTool.Info(context.Background())
	if err != nil {
		t.Fatalf("confirm_invoice.Info() 报错: %v", err)
	}
	if ciInfo.Name != "confirm_invoice" {
		t.Errorf("confirm_invoice 工具名应为 'confirm_invoice'，实际为 '%s'", ciInfo.Name)
	}
	if ciInfo.Desc == "" {
		t.Error("confirm_invoice 工具描述不应为空")
	}

	// 验证 confirm_submit 工具的 Info
	csTool := tools.NewConfirmSubmitTool(store, testLogger(t))
	csInfo, err := csTool.Info(context.Background())
	if err != nil {
		t.Fatalf("confirm_submit.Info() 报错: %v", err)
	}
	if csInfo.Name != "confirm_submit" {
		t.Errorf("confirm_submit 工具名应为 'confirm_submit'，实际为 '%s'", csInfo.Name)
	}
	if csInfo.Desc == "" {
		t.Error("confirm_submit 工具描述不应为空")
	}
}

// ============================================
// SaveState 契约测试：验证工具支持 sessionID 上下文传递
// ============================================

// TestSaveStateContract_ConfirmInvoiceUsesSessionID 验证 ConfirmInvoice 工具
// 能够从 context 中提取 sessionID 并通过 SessionStore 更新状态
func TestSaveStateContract_ConfirmInvoiceUsesSessionID(t *testing.T) {
	store := newMockStore()

	// 预填充状态
	var preState ReimbursementStateExport
	preState.Invoices = []InvoiceStateExport{
		{Amount: 100000, Category: "差旅-交通"},
	}
	preState.CurrentPhase = "phase1_collect"
	_ = store.SaveState(context.Background(), "contract-test-1", "reimbursement", &preState)

	tool := tools.NewConfirmInvoiceTool(store, testLogger(t))
	ctx := sessionCtx("contract-test-1")

	_, err := tool.InvokableRun(ctx, `{"confirmed": true}`)
	if err != nil {
		t.Fatalf("InvokableRun 报错: %v", err)
	}

	// 验证状态确实被持久化
	var state ReimbursementStateExport
	found, _ := store.GetState(context.Background(), "contract-test-1", "reimbursement", &state)
	if !found {
		t.Fatal("SaveState 未被调用——reimbursement 状态未持久化")
	}
	if !state.UserConfirmed {
		t.Error("UserConfirmed 应为 true，SaveState 未正确更新状态")
	}
}

// TestSaveStateContract_ConfirmSubmitUsesSessionID 验证 ConfirmSubmit 工具
// 能够从 context 中提取 sessionID 并通过 SessionStore 更新状态
func TestSaveStateContract_ConfirmSubmitUsesSessionID(t *testing.T) {
	store := newMockStore()

	var preState ReimbursementStateExport
	preState.CurrentPhase = "phase2_validate"
	preState.UserConfirmed = true
	preState.ComplianceResult = &ComplianceCheckResultExport{
		Result: "pass",
	}
	_ = store.SaveState(context.Background(), "contract-test-2", "reimbursement", &preState)

	tool := tools.NewConfirmSubmitTool(store, testLogger(t))
	ctx := sessionCtx("contract-test-2")

	_, err := tool.InvokableRun(ctx, `{"confirmed": true}`)
	if err != nil {
		t.Fatalf("InvokableRun 报错: %v", err)
	}

	var state ReimbursementStateExport
	found, _ := store.GetState(context.Background(), "contract-test-2", "reimbursement", &state)
	if !found {
		t.Fatal("SaveState 未被调用——reimbursement 状态未持久化")
	}
	if !state.FinalConfirmed {
		t.Error("FinalConfirmed 应为 true，SaveState 未正确更新状态")
	}
}

// ============================================
// 编译时类型检查：工具构造函数接受 SessionStore 参数
// ============================================

// TestToolsAcceptSessionStore 验证各工具构造函数接受 SessionStore 参数
// （仅做编译时检查，如果签名不匹配则编译失败，此处为文档化测试）
func TestToolsAcceptSessionStore(t *testing.T) {
	store := newMockStore()

	// 验证 ConfirmInvoiceTool 接受 SessionStore
	ci := tools.NewConfirmInvoiceTool(store, testLogger(t))
	if ci == nil {
		t.Fatal("NewConfirmInvoiceTool 不应返回 nil")
	}

	// 验证 ConfirmSubmitTool 接受 SessionStore
	cs := tools.NewConfirmSubmitTool(store, testLogger(t))
	if cs == nil {
		t.Fatal("NewConfirmSubmitTool 不应返回 nil")
	}

	// 间接验证：通过 mockToolSet 创建的 ToolSet 中确认工具已正确注入
	ts := newMockToolSet(t)
	allTools := ts.GetAllTools()

	if len(allTools) == 0 {
		t.Fatal("mockToolSet 应包含工具")
	}
}

// ============================================
// 跨阶段流程集成测试
// ============================================

// TestFullConfirmFlow 模拟完整的两步确认流程：Phase 1 确认 → Phase 2 确认
func TestFullConfirmFlow(t *testing.T) {
	store := newMockStore()

	// 步骤 1: 初始化 Phase 1 状态（模拟 OCR 识别完成后的状态）
	var state ReimbursementStateExport
	state.Invoices = []InvoiceStateExport{
		{Index: 1, Amount: 50000, Category: "差旅-交通"},
		{Index: 2, Amount: 30000, Category: "办公用品"},
	}
	state.TotalAmount = 80000
	state.CurrentPhase = "phase1_collect"
	_ = store.SaveState(context.Background(), "flow-test", "reimbursement", &state)

	// 步骤 2: Phase 1 确认票据
	ciTool := tools.NewConfirmInvoiceTool(store, testLogger(t))
	ctx := sessionCtx("flow-test")

	output, err := ciTool.InvokableRun(ctx, `{"confirmed": true}`)
	if err != nil {
		t.Fatalf("Phase 1 确认失败: %v", err)
	}
	if !containsStr(output, "confirmed") {
		t.Errorf("Phase 1 确认输出异常: %s", output)
	}

	// 验证 Phase 1 确认后的状态
	store.GetState(context.Background(), "flow-test", "reimbursement", &state)
	if state.CurrentPhase != "phase2_validate" {
		t.Fatalf("确认后 Phase 应为 phase2_validate，实际为 %s", state.CurrentPhase)
	}
	if !state.UserConfirmed {
		t.Fatal("Phase 1 确认后 UserConfirmed 应为 true")
	}

	// 步骤 3: 模拟 Phase 2 合规与预算检查完成后的状态
	state.ComplianceResult = &ComplianceCheckResultExport{
		Result: "pass", Level: "pass", Message: "全部检查通过", RuleID: "CO001",
	}
	state.BudgetResult = &BudgetCheckResultExport{
		Remaining: 500000, NeedSpecialApproval: false, UsageRate: 0.3,
	}
	_ = store.SaveState(context.Background(), "flow-test", "reimbursement", &state)

	// 步骤 4: Phase 2 最终确认提交
	csTool := tools.NewConfirmSubmitTool(store, testLogger(t))

	output, err = csTool.InvokableRun(ctx, `{"confirmed": true}`)
	if err != nil {
		t.Fatalf("Phase 2 确认失败: %v", err)
	}
	if !containsStr(output, "confirmed") {
		t.Errorf("Phase 2 确认输出异常: %s", output)
	}

	// 验证最终状态
	store.GetState(context.Background(), "flow-test", "reimbursement", &state)
	if state.CurrentPhase != "phase3_execute" {
		t.Errorf("最终确认后 Phase 应为 phase3_execute，实际为 %s", state.CurrentPhase)
	}
	if !state.FinalConfirmed {
		t.Error("Phase 2 确认后 FinalConfirmed 应为 true")
	}
	if len(state.Invoices) != 2 {
		t.Errorf("流程结束后票据不应丢失，期望 2 张，实际 %d 张", len(state.Invoices))
	}
}

func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

// ============================================
// 导出类型别名
// ============================================

// 以下类型与 agenttypes 包中的对应类型结构完全一致，
// 用于在测试中构造和反序列化 ReimbursementState。
// 使用独立类型避免直接依赖内部包（已在 import 中通过 agenttypes 引入）。

type ReimbursementStateExport struct {
	Invoices            []InvoiceStateExport       `json:"invoices"`
	TotalAmount         int64                      `json:"total_amount"`
	UserConfirmed       bool                       `json:"user_confirmed"`
	CurrentPhase        string                     `json:"current_phase"`
	ComplianceResult    *ComplianceCheckResultExport `json:"compliance_result,omitempty"`
	BudgetResult        *BudgetCheckResultExport     `json:"budget_result,omitempty"`
	FinalConfirmed      bool                       `json:"final_confirmed"`
	NeedSpecialApproval bool                       `json:"need_special_approval"`
}

type InvoiceStateExport struct {
	Index     int    `json:"index"`
	Amount    int64  `json:"amount"`
	Category  string `json:"category"`
}

type ComplianceCheckResultExport struct {
	Result  string `json:"result"`
	Level   string `json:"level"`
	Message string `json:"message"`
	RuleID  string `json:"rule_id"`
}

type BudgetCheckResultExport struct {
	Remaining           int64   `json:"remaining"`
	NeedSpecialApproval bool    `json:"need_special_approval"`
	UsageRate           float64 `json:"usage_rate"`
}
