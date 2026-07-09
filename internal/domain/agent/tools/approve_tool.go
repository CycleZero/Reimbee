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
		"审批通过报销单。仅审批当前登录审批人对应的审批记录，全部通过后报销单状态变为 approved。",
		func(ctx context.Context, input ApproveInput) (ApproveOutput, error) {
			approverName := getApproverName(ctx)
			if approverName == "" {
				return ApproveOutput{}, fmt.Errorf("无法获取审批人身份信息")
			}

			rm, err := reimbursementBiz.Approve(input.ReimbursementID, approverName)
			if err != nil {
				return ApproveOutput{}, fmt.Errorf("审批失败: %w", err)
			}
			logger.Info("审批完成",
				zap.Uint("ID", input.ReimbursementID),
				zap.String("审批人", approverName),
				zap.String("状态", rm.Status))
			return ApproveOutput{Status: rm.Status}, nil
		},
	)
	if err != nil {
		panic("创建审批工具失败: " + err.Error())
	}
	return &ApproveTool{NewInterruptable(base, "确认审批通过此报销单？")}
}

type approverCtxKey struct{}

// WithApproverName 将审批人姓名注入 context（由 Run 调用）
func WithApproverName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, approverCtxKey{}, name)
}

// getApproverName 从 context 中提取审批人姓名
func getApproverName(ctx context.Context) string {
	name, _ := ctx.Value(approverCtxKey{}).(string)
	return name
}
