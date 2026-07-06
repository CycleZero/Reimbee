package phase

import (
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/model"
)

// ============================================
// Phase 1 → Phase 2 护卫条件
// ============================================

// Phase1Guard Phase 1 → Phase 2 的护卫条件
// 条件：≥1张票据已上传 AND 每张票据有金额 AND 用户确认了票据信息
func Phase1Guard(state *agent.ReimbursementState) *agent.GuardResult {
	if state == nil {
		return &agent.GuardResult{Passed: false, Reason: "状态为空", Message: "系统异常，请重新开始"}
	}

	if len(state.Invoices) == 0 {
		return &agent.GuardResult{
			Passed:  false,
			Reason:  "无票据",
			Message: "请至少上传一张票据图片作为报销凭证",
		}
	}

	for i, inv := range state.Invoices {
		if inv.Amount <= 0 {
			return &agent.GuardResult{
				Passed:  false,
				Reason:  fmt.Sprintf("票据 %d 缺少金额", i+1),
				Message: fmt.Sprintf("第 %d 张票据缺少金额，请补充", i+1),
			}
		}
		if inv.Category == "" {
			return &agent.GuardResult{
				Passed:  false,
				Reason:  fmt.Sprintf("票据 %d 缺少类别", i+1),
				Message: fmt.Sprintf("第 %d 张票据缺少费用类别，请选择（如：差旅-交通、招待费等）", i+1),
			}
		}
	}

	if !state.UserConfirmed {
		return &agent.GuardResult{
			Passed:  false,
			Reason:  "用户未确认票据",
			Message: "请先确认票据信息无误后再进入下一步",
		}
	}

	return &agent.GuardResult{Passed: true}
}

// Phase2Guard Phase 2 → Phase 3 的护卫条件
func Phase2Guard(state *agent.ReimbursementState) *agent.GuardResult {
	if state == nil {
		return &agent.GuardResult{Passed: false, Reason: "状态为空", Message: "系统异常，请重新开始"}
	}

	if state.ComplianceResult == nil {
		return &agent.GuardResult{
			Passed:  false,
			Reason:  "合规检查未完成",
			Message: "尚未完成合规检查，请稍候",
		}
	}

	if state.ComplianceResult.Result == model.CheckResultError {
		return &agent.GuardResult{
			Passed:  false,
			Reason:  "合规检查不通过",
			Message: state.ComplianceResult.Message,
		}
	}

	if state.BudgetResult != nil && state.BudgetResult.NeedSpecialApproval {
		if !state.FinalConfirmed {
			return &agent.GuardResult{
				Passed:  false,
				Reason:  "预算不足，等待用户确认",
				Message: "当前预算不足，将触发特殊审批流程。是否确认提交？",
			}
		}
	}

	if !state.FinalConfirmed {
		return &agent.GuardResult{
			Passed:  false,
			Reason:  "用户未最终确认",
			Message: "请先确认提交（提交后不可撤销，报销单将发送给审批人）",
		}
	}

	return &agent.GuardResult{Passed: true}
}

// Phase3Complete 检查 Phase 3 是否已完成
func Phase3Complete(state *agent.ReimbursementState) bool {
	if state == nil {
		return false
	}
	return state.CurrentPhase == "phase3_execute"
}
