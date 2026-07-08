package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

type SubmitReimbInput struct {
	ReimbursementID uint `json:"reimbursement_id"`
}

type SubmitReimbOutput struct {
	ReimbursementNo     string `json:"reimbursement_no"`
	Status              string `json:"status"`
	NeedSpecialApproval bool   `json:"need_special_approval"`
}

type SubmitReimbTool struct{ tools.Tool }

func NewSubmitReimbTool(reimbursementBiz *reimbursement.ReimbursementBiz, store infra.StateStore, logger *log.Logger) *SubmitReimbTool {
	base, err := tools.NewFunc[SubmitReimbInput, SubmitReimbOutput](
		"submit_reimbursement",
		"提交报销单进入审批流程。调用后将冻结部门预算、创建审批链。提交后不可撤销。",
		func(ctx context.Context, input SubmitReimbInput) (SubmitReimbOutput, error) {
			sid := getSessionID(ctx)

			// 读取报销状态
			var state types.ReimbursementState
			store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)

			logger.Info("执行报销单提交",
				zap.Uint("报销单ID", input.ReimbursementID),
				zap.Int64("总金额(分)", state.TotalAmount))

			rm, err := reimbursementBiz.Submit(input.ReimbursementID, state.TotalAmount)
			if err != nil {
				return SubmitReimbOutput{}, fmt.Errorf("提交失败: %w", err)
			}

			logger.Info("报销单提交成功",
				zap.String("单号", rm.ReimbursementNo),
				zap.String("状态", rm.Status))

			return SubmitReimbOutput{
				ReimbursementNo:     rm.ReimbursementNo,
				Status:              rm.Status,
				NeedSpecialApproval: rm.NeedSpecialApproval,
			}, nil
		},
	)
	if err != nil {
		panic("创建submit_reimbursement工具失败: " + err.Error())
	}
	logger.Info("提交报销单工具初始化完成（含Interruptable包装）")
	return &SubmitReimbTool{NewRoleGuard(NewInterruptable(base, "确认提交报销单？提交后不可撤销。"), "employee")}
}
