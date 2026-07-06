package phase_test

import (
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/internal/domain/agent/phase"
	"github.com/CycleZero/Reimbee/model"
)

// ============================================
// Phase1Guard 测试
// ============================================

// TestPhase1Guard_Pass 验证票据就绪且用户确认后通过护卫条件
func TestPhase1Guard_Pass(t *testing.T) {
	state := &agent.ReimbursementState{
		Invoices: []agent.InvoiceState{
			{
				Index:    1,
				Amount:   10000,       // 100.00 元
				Category: "差旅-交通",
				UserConfirmed: true,
			},
		},
		UserConfirmed: true,
	}

	result := phase.Phase1Guard(state)

	if !result.Passed {
		t.Errorf("完整的票据应通过 Phase1 护卫，实际未通过，原因: %s", result.Reason)
	}
}

// TestPhase1Guard_NoInvoices 验证无票据时护卫失败
func TestPhase1Guard_NoInvoices(t *testing.T) {
	state := &agent.ReimbursementState{
		Invoices:      []agent.InvoiceState{},
		UserConfirmed: false,
	}

	result := phase.Phase1Guard(state)

	if result.Passed {
		t.Error("无票据时应不通过 Phase1 护卫")
	}
	if result.Reason != "无票据" {
		t.Errorf("失败原因应为'无票据'，实际为: %s", result.Reason)
	}
}

// TestPhase1Guard_NoAmount 验证票据金额为 0 时护卫失败
func TestPhase1Guard_NoAmount(t *testing.T) {
	state := &agent.ReimbursementState{
		Invoices: []agent.InvoiceState{
			{
				Index:    1,
				Amount:   0, // 金额缺失
				Category: "差旅-交通",
			},
		},
	}

	result := phase.Phase1Guard(state)

	if result.Passed {
		t.Error("票据缺少金额时应不通过 Phase1 护卫")
	}
	if result.Message == "" {
		t.Error("失败时应返回用户提示消息")
	}
}

// TestPhase1Guard_NoCategory 验证票据类别为空时护卫失败
func TestPhase1Guard_NoCategory(t *testing.T) {
	state := &agent.ReimbursementState{
		Invoices: []agent.InvoiceState{
			{
				Index:  1,
				Amount: 10000,
				Category: "", // 类别缺失
			},
		},
	}

	result := phase.Phase1Guard(state)

	if result.Passed {
		t.Error("票据缺少类别时应不通过 Phase1 护卫")
	}
}

// TestPhase1Guard_NotConfirmed 验证票据完整但用户未确认时护卫失败
func TestPhase1Guard_NotConfirmed(t *testing.T) {
	state := &agent.ReimbursementState{
		Invoices: []agent.InvoiceState{
			{
				Index:    1,
				Amount:   10000,
				Category: "招待费",
				UserConfirmed: true, // 单张确认
			},
		},
		UserConfirmed: false, // 全局未确认
	}

	result := phase.Phase1Guard(state)

	if result.Passed {
		t.Error("用户未全局确认时应不通过 Phase1 护卫")
	}
}

// TestPhase1Guard_NilState 验证 nil 状态时护卫失败
func TestPhase1Guard_NilState(t *testing.T) {
	result := phase.Phase1Guard(nil)

	if result.Passed {
		t.Error("nil 状态时应不通过 Phase1 护卫")
	}
	if result.Reason != "状态为空" {
		t.Errorf("nil 状态的失败原因应为'状态为空'，实际为: %s", result.Reason)
	}
}

// TestPhase1Guard_MultipleInvoices 验证多张票据全部合法时通过
func TestPhase1Guard_MultipleInvoices(t *testing.T) {
	state := &agent.ReimbursementState{
		Invoices: []agent.InvoiceState{
			{Index: 1, Amount: 10000, Category: "差旅-交通", UserConfirmed: true},
			{Index: 2, Amount: 5000, Category: "招待费", UserConfirmed: true},
			{Index: 3, Amount: 2000, Category: "办公用品", UserConfirmed: true},
		},
		UserConfirmed: true,
	}

	result := phase.Phase1Guard(state)

	if !result.Passed {
		t.Errorf("多张完整票据应通过 Phase1 护卫，实际未通过，原因: %s", result.Reason)
	}
}

// TestPhase1Guard_SecondInvoiceMissingCategory 验证第二张票据缺类别时护卫失败
func TestPhase1Guard_SecondInvoiceMissingCategory(t *testing.T) {
	state := &agent.ReimbursementState{
		Invoices: []agent.InvoiceState{
			{Index: 1, Amount: 10000, Category: "差旅-交通", UserConfirmed: true},
			{Index: 2, Amount: 5000, Category: "", UserConfirmed: true}, // 第二张缺类别
		},
		UserConfirmed: true,
	}

	result := phase.Phase1Guard(state)

	if result.Passed {
		t.Error("第二张票据缺少类别，应不通过 Phase1 护卫")
	}
}

// ============================================
// Phase2Guard 测试
// ============================================

// TestPhase2Guard_Pass 验证合规通过且用户最终确认后通过护卫条件
func TestPhase2Guard_Pass(t *testing.T) {
	state := &agent.ReimbursementState{
		ComplianceResult: &agent.ComplianceCheckResult{
			Result:  model.CheckResultPass,
			Message: "合规检查通过",
		},
		FinalConfirmed: true,
	}

	result := phase.Phase2Guard(state)

	if !result.Passed {
		t.Errorf("合规通过且用户确认后应通过 Phase2 护卫，实际未通过，原因: %s", result.Reason)
	}
}

// TestPhase2Guard_NoCompliance 验证合规检查未完成时护卫失败
func TestPhase2Guard_NoCompliance(t *testing.T) {
	state := &agent.ReimbursementState{
		ComplianceResult: nil,
	}

	result := phase.Phase2Guard(state)

	if result.Passed {
		t.Error("合规检查未完成时应不通过 Phase2 护卫")
	}
	if result.Reason != "合规检查未完成" {
		t.Errorf("失败原因应为'合规检查未完成'，实际为: %s", result.Reason)
	}
}

// TestPhase2Guard_ComplianceError 验证合规检查报错时护卫失败
func TestPhase2Guard_ComplianceError(t *testing.T) {
	state := &agent.ReimbursementState{
		ComplianceResult: &agent.ComplianceCheckResult{
			Result:  model.CheckResultError,
			Message: "票据金额超出公司标准上限 300%",
		},
	}

	result := phase.Phase2Guard(state)

	if result.Passed {
		t.Error("合规检查不通过时应不通过 Phase2 护卫")
	}
	if result.Message == "" {
		t.Error("失败时应返回合规错误消息")
	}
}

// TestPhase2Guard_ComplianceWarning_Pass 验证合规警告且用户确认后通过
func TestPhase2Guard_ComplianceWarning_Pass(t *testing.T) {
	state := &agent.ReimbursementState{
		ComplianceResult: &agent.ComplianceCheckResult{
			Result:  model.CheckResultWarning,
			Message: "住宿费略超标准，已标记供审批人参考",
		},
		FinalConfirmed: true,
	}

	result := phase.Phase2Guard(state)

	if !result.Passed {
		t.Errorf("合规警告但用户确认后应通过 Phase2 护卫，实际未通过，原因: %s", result.Reason)
	}
}

// TestPhase2Guard_NotFinalConfirmed 验证合规通过但用户未最终确认时护卫失败
func TestPhase2Guard_NotFinalConfirmed(t *testing.T) {
	state := &agent.ReimbursementState{
		ComplianceResult: &agent.ComplianceCheckResult{
			Result:  model.CheckResultPass,
			Message: "合规检查通过",
		},
		FinalConfirmed: false,
	}

	result := phase.Phase2Guard(state)

	if result.Passed {
		t.Error("用户未最终确认时应不通过 Phase2 护卫")
	}
	if result.Reason != "用户未最终确认" {
		t.Errorf("失败原因应为'用户未最终确认'，实际为: %s", result.Reason)
	}
}

// TestPhase2Guard_NilState 验证 nil 状态时护卫失败
func TestPhase2Guard_NilState(t *testing.T) {
	result := phase.Phase2Guard(nil)

	if result.Passed {
		t.Error("nil 状态时应不通过 Phase2 护卫")
	}
	if result.Reason != "状态为空" {
		t.Errorf("nil 状态的失败原因应为'状态为空'，实际为: %s", result.Reason)
	}
}

// TestPhase2Guard_BudgetDeficit_NotConfirmed 验证预算不足且用户未确认时护卫失败
func TestPhase2Guard_BudgetDeficit_NotConfirmed(t *testing.T) {
	state := &agent.ReimbursementState{
		ComplianceResult: &agent.ComplianceCheckResult{
			Result:  model.CheckResultPass,
			Message: "合规检查通过",
		},
		BudgetResult: &agent.BudgetCheckResult{
			Remaining:           5000,
			NeedSpecialApproval: true,
			UsageRate:           0.95,
		},
		FinalConfirmed: false,
	}

	result := phase.Phase2Guard(state)

	if result.Passed {
		t.Error("预算不足且用户未确认时应不通过 Phase2 护卫")
	}
}

// TestPhase2Guard_BudgetDeficit_Confirmed 验证预算不足但用户确认后通过
func TestPhase2Guard_BudgetDeficit_Confirmed(t *testing.T) {
	state := &agent.ReimbursementState{
		ComplianceResult: &agent.ComplianceCheckResult{
			Result:  model.CheckResultPass,
			Message: "合规检查通过",
		},
		BudgetResult: &agent.BudgetCheckResult{
			Remaining:           5000,
			NeedSpecialApproval: true,
			UsageRate:           0.95,
		},
		FinalConfirmed: true,
	}

	result := phase.Phase2Guard(state)

	if !result.Passed {
		t.Errorf("预算不足但用户已确认时应通过 Phase2 护卫，实际未通过，原因: %s", result.Reason)
	}
}

// ============================================
// Phase3Complete 测试
// ============================================

// TestPhase3Complete_NilState 验证 nil 状态返回 false
func TestPhase3Complete_NilState(t *testing.T) {
	if phase.Phase3Complete(nil) {
		t.Error("nil 状态时 Phase3Complete 应返回 false")
	}
}

// TestPhase3Complete_NotInPhase3 验证非 phase3 状态返回 false
func TestPhase3Complete_NotInPhase3(t *testing.T) {
	state := &agent.ReimbursementState{
		CurrentPhase: "phase2_validate",
	}

	if phase.Phase3Complete(state) {
		t.Error("非 phase3_execute 状态时 Phase3Complete 应返回 false")
	}
}

// TestPhase3Complete_InPhase3 验证 phase3 状态返回 true
func TestPhase3Complete_InPhase3(t *testing.T) {
	state := &agent.ReimbursementState{
		CurrentPhase: "phase3_execute",
	}

	if !phase.Phase3Complete(state) {
		t.Error("phase3_execute 状态时 Phase3Complete 应返回 true")
	}
}
