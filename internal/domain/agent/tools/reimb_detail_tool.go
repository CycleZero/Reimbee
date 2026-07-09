package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

type ReimbDetailInput struct {
	ReimbursementID uint `json:"reimbursement_id"`
}

type ReimbDetailOutput struct {
	No          string `json:"no"`
	Status      string `json:"status"`
	TotalAmount int64  `json:"total_amount"`
	SubmitNote  string `json:"submit_note"`
	CreatedAt   string `json:"created_at"`
}

type ReimbDetailTool struct{ tools.Tool }

func NewReimbDetailTool(reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *ReimbDetailTool {
	t, err := tools.NewFunc[ReimbDetailInput, ReimbDetailOutput](
		ToolReimbDetail,
		"查询指定报销单的详细信息，包括单号、状态、金额、事由、创建时间。",
		func(ctx context.Context, input ReimbDetailInput) (ReimbDetailOutput, error) {
			rm, err := reimbursementBiz.GetByID(input.ReimbursementID)
			if err != nil {
				return ReimbDetailOutput{}, fmt.Errorf("查询报销单失败: %w", err)
			}
			logger.Debug("报销单查询完成", zap.Uint("ID", input.ReimbursementID))
			return ReimbDetailOutput{
				No:          rm.ReimbursementNo,
				Status:      rm.Status,
				TotalAmount: rm.TotalAmount,
				SubmitNote:  rm.SubmitNote,
				CreatedAt:   rm.CreatedAt.Format("2006-01-02 15:04:05"),
			}, nil
		},
	)
	if err != nil {
		panic("创建报销单详情工具失败: " + err.Error())
	}
	return &ReimbDetailTool{t}
}
