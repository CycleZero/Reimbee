package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

type PendingInput struct{}

type PendingItem struct {
	ID          uint   `json:"id"`
	No          string `json:"no"`
	SubmitNote  string `json:"submit_note"`
	TotalAmount int64  `json:"total_amount"`
	CreatedAt   string `json:"created_at"`
}

type PendingOutput struct {
	List []PendingItem `json:"list"`
}

type PendingTool struct{ tools.Tool }

func NewPendingTool(reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *PendingTool {
	t, err := tools.NewFunc[PendingInput, PendingOutput](
		"list_pending",
		"查询待审批的报销单列表。返回报销单ID、单号、事由、金额、创建时间。",
		func(ctx context.Context, _ PendingInput) (PendingOutput, error) {
			rms, err := reimbursementBiz.ListPending()
			if err != nil {
				return PendingOutput{}, fmt.Errorf("查询待审批列表失败: %w", err)
			}
			items := make([]PendingItem, 0, len(rms))
			for _, rm := range rms {
				items = append(items, PendingItem{
					ID:          rm.ID,
					No:          rm.ReimbursementNo,
					SubmitNote:  rm.SubmitNote,
					TotalAmount: rm.TotalAmount,
					CreatedAt:   rm.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			logger.Info("待审批列表查询完成", zap.Int("数量", len(items)))
			return PendingOutput{List: items}, nil
		},
	)
	if err != nil {
		panic("创建待审批列表工具失败: " + err.Error())
	}
	return &PendingTool{t}
}
