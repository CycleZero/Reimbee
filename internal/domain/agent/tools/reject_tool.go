package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

type RejectInput struct {
	ReimbursementID uint   `json:"reimbursement_id"`
	Reason          string `json:"reason"`
}

type RejectOutput struct {
	Status string `json:"status"`
}

type RejectTool struct{ tools.Tool }

func NewRejectTool(reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *RejectTool {
	t, err := tools.NewFunc[RejectInput, RejectOutput](
		"reject_reimbursement",
		"驳回报销单，需填写驳回理由。",
		func(ctx context.Context, input RejectInput) (RejectOutput, error) {
			rm, err := reimbursementBiz.Reject(input.ReimbursementID, input.Reason)
			if err != nil {
				return RejectOutput{}, fmt.Errorf("驳回失败: %w", err)
			}
			logger.Info("报销单已驳回", zap.Uint("ID", input.ReimbursementID), zap.String("理由", input.Reason))
			return RejectOutput{Status: rm.Status}, nil
		},
	)
	if err != nil {
		panic("创建驳回工具失败: " + err.Error())
	}
	return &RejectTool{t}
}
