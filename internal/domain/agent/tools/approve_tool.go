package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

type ApproveInput struct{ ReimbursementID uint `json:"reimbursement_id"` }

type ApproveOutput struct {
	Status string `json:"status"`
}

type ApproveTool struct{ tools.Tool }

func NewApproveTool(reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *ApproveTool {
	base, err := tools.NewFunc[ApproveInput, ApproveOutput](
		"approve_reimbursement",
		"审批通过报销单。调用后将更新报销单状态为已通过。需审批人显式确认。",
		func(ctx context.Context, input ApproveInput) (ApproveOutput, error) {
			rm, err := reimbursementBiz.Approve(input.ReimbursementID)
			if err != nil {
				return ApproveOutput{}, fmt.Errorf("审批失败: %w", err)
			}
			logger.Info("审批通过", zap.Uint("ID", input.ReimbursementID), zap.String("单号", rm.ReimbursementNo))
			return ApproveOutput{Status: rm.Status}, nil
		},
	)
	if err != nil {
		panic("创建审批工具失败: " + err.Error())
	}
	return &ApproveTool{NewRoleGuard(NewInterruptable(base, "确认审批通过此报销单？"), "approver", "admin")}
}
