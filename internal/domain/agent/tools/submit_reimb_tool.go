// Package tools 提交报销单工具（v4 简化）
// v4 中 Interrupt 由 TurnLoop 层通过 Checkpoint+GenResume 管理，
// 工具本身保持简洁，仅负责提交逻辑。
package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"go.uber.org/zap"
)

type SubmitReimbInput struct {
	Confirmed bool `json:"confirmed" jsonschema:"required" jsonschema_description:"用户是否最终确认提交"`
}

type SubmitReimbOutput struct {
	ReimbursementNo     string `json:"reimbursement_no"`
	Status              string `json:"status"`
	NeedSpecialApproval bool   `json:"need_special_approval"`
}

type SubmitReimbTool struct{ tool.InvokableTool }

func NewSubmitReimbTool(reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *SubmitReimbTool {
	t, err := utils.InferTool[SubmitReimbInput, SubmitReimbOutput](
		"submit_reimbursement",
		"提交报销单进入审批流程。调用此工具表示用户已最终确认提交，执行后将冻结部门预算、创建审批链。提交后不可撤销。",
		func(ctx context.Context, input SubmitReimbInput) (SubmitReimbOutput, error) {
			if !input.Confirmed {
				return SubmitReimbOutput{}, fmt.Errorf("用户未确认提交")
			}

			var state types.ReimbursementState
			if raw, ok := ctx.Value(types.StateContextKey{}).(*types.ReimbursementState); ok {
				state = *raw
			}

			logger.Info("执行报销单提交",
				zap.Int("票据数", len(state.Invoices)),
				zap.Int64("总金额(分)", state.TotalAmount))

			rm, err := reimbursementBiz.Submit(state.ReimbursementID, state.TotalAmount)
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
	logger.Info("提交报销单工具初始化完成")
	return &SubmitReimbTool{t}
}
