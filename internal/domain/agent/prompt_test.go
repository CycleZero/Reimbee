package agent_test

import (
	"strings"
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
)

// ============================================
// BuildSystemPrompt 测试
// ============================================

// TestBuildSystemPrompt_Phase1 验证 Phase 1 提示词包含"信息收集阶段"
func TestBuildSystemPrompt_Phase1(t *testing.T) {
	prompt := agent.BuildSystemPrompt("phase1_collect", nil)

	if !strings.Contains(prompt, "信息收集阶段") {
		t.Error("Phase 1 提示词应包含'信息收集阶段'")
	}
	if !strings.Contains(prompt, "Reimbee") {
		t.Error("系统提示词应包含助手名称'Reimbee'")
	}
}

// TestBuildSystemPrompt_Phase2 验证 Phase 2 提示词包含"校验确认阶段"
func TestBuildSystemPrompt_Phase2(t *testing.T) {
	prompt := agent.BuildSystemPrompt("phase2_validate", nil)

	if !strings.Contains(prompt, "校验确认阶段") {
		t.Error("Phase 2 提示词应包含'校验确认阶段'")
	}
}

// TestBuildSystemPrompt_Phase3 验证 Phase 3 提示词包含"执行提交阶段"
func TestBuildSystemPrompt_Phase3(t *testing.T) {
	prompt := agent.BuildSystemPrompt("phase3_execute", nil)

	if !strings.Contains(prompt, "执行提交阶段") {
		t.Error("Phase 3 提示词应包含'执行提交阶段'")
	}
}

// TestBuildSystemPrompt_UnknownPhase 验证未知阶段的提示词回退
func TestBuildSystemPrompt_UnknownPhase(t *testing.T) {
	prompt := agent.BuildSystemPrompt("unknown_phase", nil)

	if !strings.Contains(prompt, "报销相关操作") {
		t.Error("未知阶段提示词应包含通用指引'报销相关操作'")
	}
}

// TestBuildSystemPrompt_WithState 验证带状态的提示词包含状态摘要信息
func TestBuildSystemPrompt_WithState(t *testing.T) {
	state := &agent.ReimbursementState{
		EmployeeName: "张三",
		EmployeeID:   "EMP001",
		TotalAmount:  15000, // 150.00 元
		Invoices: []agent.InvoiceState{
			{
				Index:     1,
				Category:  "差旅-交通",
				Amount:    10000, // 100.00 元
				UserConfirmed: true,
			},
		},
		ComplianceResult: &agent.ComplianceCheckResult{
			Result:  "pass",
			Message: "所有票据符合公司报销标准",
		},
	}

	prompt := agent.BuildSystemPrompt("phase2_validate", state)

	// 验证状态摘要被注入
	if !strings.Contains(prompt, "张三") {
		t.Error("提示词应包含申请人姓名'张三'")
	}
	if !strings.Contains(prompt, "EMP001") {
		t.Error("提示词应包含员工工号'EMP001'")
	}
	if !strings.Contains(prompt, "150.00") {
		t.Error("提示词应包含总金额 150.00 元")
	}
	if !strings.Contains(prompt, "差旅-交通") {
		t.Error("提示词应包含票据类别'差旅-交通'")
	}
	if !strings.Contains(prompt, "pass") {
		t.Error("提示词应包含合规检查结果'pass'")
	}
}

// ============================================
// BuildIntentClassifyPrompt 测试
// ============================================

// TestBuildIntentClassifyPrompt 验证意图分类提示词包含用户消息
func TestBuildIntentClassifyPrompt(t *testing.T) {
	userMsg := "我要报销一张出租车发票"
	prompt := agent.BuildIntentClassifyPrompt(userMsg)

	if !strings.Contains(prompt, userMsg) {
		t.Error("意图分类提示词应包含用户输入消息")
	}
	if !strings.Contains(prompt, "new_reimbursement") {
		t.Error("意图分类提示词应包含意图类别'new_reimbursement'")
	}
	if !strings.Contains(prompt, "confidence") {
		t.Error("意图分类提示词应包含置信度字段'confidence'")
	}
}

// ============================================
// BuildStateSummary 测试
// ============================================

// TestBuildStateSummary_Empty 验证 nil 状态返回空字符串
func TestBuildStateSummary_Empty(t *testing.T) {
	summary := agent.BuildStateSummary(nil)

	if summary != "" {
		t.Errorf("nil 状态应返回空字符串，实际为: %s", summary)
	}
}

// TestBuildStateSummary_WithInvoices 验证带票据的状态摘要包含票据详情
func TestBuildStateSummary_WithInvoices(t *testing.T) {
	state := &agent.ReimbursementState{
		ReimbursementNo: "REIMB-2026-0001",
		EmployeeName:    "李四",
		EmployeeID:      "EMP002",
		TotalAmount:     25000, // 250.00 元
		Invoices: []agent.InvoiceState{
			{Index: 1, Category: "差旅-交通", Amount: 12000, UserConfirmed: true},
			{Index: 2, Category: "招待费", Amount: 13000, UserConfirmed: false},
		},
	}

	summary := agent.BuildStateSummary(state)

	checks := []string{
		"REIMB-2026-0001",
		"李四",
		"EMP002",
		"250.00",
		"差旅-交通",
		"120.00",
		"招待费",
		"130.00",
		"✓",
	}

	for _, check := range checks {
		if !strings.Contains(summary, check) {
			t.Errorf("状态摘要应包含 '%s'，但未找到", check)
		}
	}
}

// TestBuildStateSummary_WithCompliance 验证包含合规检查结果的状态摘要
func TestBuildStateSummary_WithCompliance(t *testing.T) {
	state := &agent.ReimbursementState{
		ComplianceResult: &agent.ComplianceCheckResult{
			Result:  "warning",
			Message: "差旅住宿超出标准 200 元",
		},
	}

	summary := agent.BuildStateSummary(state)

	if !strings.Contains(summary, "warning") {
		t.Error("状态摘要应包含合规检查结果 'warning'")
	}
	if !strings.Contains(summary, "差旅住宿超出标准 200 元") {
		t.Error("状态摘要应包含合规检查消息")
	}
}

// TestBuildStateSummary_ModifiedInvoices 验证包含"已修正"标记的状态摘要
func TestBuildStateSummary_ModifiedInvoices(t *testing.T) {
	state := &agent.ReimbursementState{
		TotalAmount: 10000,
		Invoices: []agent.InvoiceState{
			{
				Index:        1,
				Category:     "招待费",
				Amount:       8000,
				OCRRawAmount: 12000,
				IsModified:   true,
			},
		},
	}

	summary := agent.BuildStateSummary(state)

	if !strings.Contains(summary, "已修正") {
		t.Error("修改过的票据应标记为'已修正'")
	}
	if !strings.Contains(summary, "120.00") {
		t.Error("状态摘要应包含 OCR 原始金额 120.00")
	}
}

// ============================================
// BuildModifiedInvoicesWarning 测试
// ============================================

// TestBuildModifiedInvoicesWarning 验证包含修正票据时的警告信息
func TestBuildModifiedInvoicesWarning(t *testing.T) {
	state := &agent.ReimbursementState{
		Invoices: []agent.InvoiceState{
			{
				Index:        1,
				Category:     "差旅-交通",
				Amount:       10000,
				OCRRawAmount: 8000,
				IsModified:   true,
				ModifyReason:  "OCR 识别金额偏低",
			},
			{
				Index:        2,
				Category:     "招待费",
				Amount:       5000,
				OCRRawAmount: 6000,
				IsModified:   true,
			},
		},
	}

	warning := agent.BuildModifiedInvoicesWarning(state)

	if !strings.Contains(warning, "2 项票据") {
		t.Error("警告应包含修正票据数量'2 项票据'")
	}
	if !strings.Contains(warning, "差旅-交通") {
		t.Error("警告应包含票据类别'差旅-交通'")
	}
	if !strings.Contains(warning, "招待费") {
		t.Error("警告应包含票据类别'招待费'")
	}
	if !strings.Contains(warning, "OCR 识别金额偏低") {
		t.Error("警告应包含修正原因'OCR 识别金额偏低'")
	}
	if !strings.Contains(warning, "80.00") {
		t.Error("警告应包含 OCR 原始金额 80.00 元")
	}
	if !strings.Contains(warning, "100.00") {
		t.Error("警告应包含修正后金额 100.00 元")
	}
}

// TestBuildModifiedInvoicesWarning_None 验证无修正票据时返回空字符串
func TestBuildModifiedInvoicesWarning_None(t *testing.T) {
	state := &agent.ReimbursementState{
		Invoices: []agent.InvoiceState{
			{Index: 1, Category: "差旅-交通", Amount: 10000, IsModified: false},
			{Index: 2, Category: "招待费", Amount: 5000, IsModified: false},
		},
	}

	warning := agent.BuildModifiedInvoicesWarning(state)

	if warning != "" {
		t.Errorf("无修正票据时应返回空字符串，实际为: %s", warning)
	}
}

// TestBuildModifiedInvoicesWarning_NilState 验证 nil 状态返回空字符串
func TestBuildModifiedInvoicesWarning_NilState(t *testing.T) {
	warning := agent.BuildModifiedInvoicesWarning(nil)

	if warning != "" {
		t.Errorf("nil 状态时应返回空字符串，实际为: %s", warning)
	}
}

// TestBuildModifiedInvoicesWarning_EmptyInvoices 验证空票据列表返回空字符串
func TestBuildModifiedInvoicesWarning_EmptyInvoices(t *testing.T) {
	state := &agent.ReimbursementState{
		Invoices: []agent.InvoiceState{},
	}

	warning := agent.BuildModifiedInvoicesWarning(state)

	if warning != "" {
		t.Errorf("空票据列表时应返回空字符串，实际为: %s", warning)
	}
}

// ============================================
// BuildGeneralChatPrompt 测试
// ============================================

// TestBuildGeneralChatPrompt 验证通用对话提示词非空
func TestBuildGeneralChatPrompt(t *testing.T) {
	prompt := agent.BuildGeneralChatPrompt()

	if prompt == "" {
		t.Error("通用对话提示词不应为空")
	}
	if !strings.Contains(prompt, "Reimbee") {
		t.Error("通用对话提示词应包含助手名称'Reimbee'")
	}
	if !strings.Contains(prompt, "报销") {
		t.Error("通用对话提示词应包含'报销'关键字")
	}
}
