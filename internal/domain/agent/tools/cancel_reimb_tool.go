package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

type CancelReimbInput struct {
	ReimbursementID uint `json:"reimbursement_id"`
}

type CancelReimbOutput struct {
	Status string `json:"status"`
}

type CancelReimbTool struct{ tools.Tool }

func NewCancelReimbTool(reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *CancelReimbTool {
	base, err := tools.NewFunc[CancelReimbInput, CancelReimbOutput](
		"cancel_reimbursement",
		"取消报销单草稿（仅 draft 状态可取消）。取消后不可恢复，需用户显式确认。",
		func(ctx context.Context, input CancelReimbInput) (CancelReimbOutput, error) {
			rm, err := reimbursementBiz.GetByID(input.ReimbursementID)
			if err != nil {
				return CancelReimbOutput{}, fmt.Errorf("查询报销单失败: %w", err)
			}
			if rm.Status != "draft" {
				return CancelReimbOutput{}, fmt.Errorf("只有草稿状态的报销单可以取消，当前状态: %s", rm.Status)
			}
			// 软删除：设为已驳回
			if _, err := reimbursementBiz.Reject(input.ReimbursementID); err != nil {
				return CancelReimbOutput{}, fmt.Errorf("取消报销单失败: %w", err)
			}
			logger.Info("报销单已取消", zap.Uint("ID", input.ReimbursementID), zap.String("单号", rm.ReimbursementNo))
			return CancelReimbOutput{Status: "cancelled"}, nil
		},
	)
	if err != nil {
		panic("创建取消报销单工具失败: " + err.Error())
	}
	logger.Info("取消报销单工具初始化完成（含Interruptable包装）")
	return &CancelReimbTool{NewInterruptable(base, "确认取消此报销单？取消后不可恢复。")}
}
