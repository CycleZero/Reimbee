package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// ListInvoicesInput 列出票据汇总的输入参数
type ListInvoicesInput struct{}

// ListInvoicesOutput 列出票据汇总的输出（按明细→票据层级）
type ListInvoicesOutput struct {
	// Items 已确认的报销明细列表
	Items []ItemSummary `json:"items"`
	// PendingReceipts 尚未归类的票据
	PendingReceipts []ReceiptSummary `json:"pending_receipts"`
	// TotalCount 票据总数（含已归类和待归类）
	TotalCount int `json:"total_count"`
	// TotalAmount 票据总金额（元）
	TotalAmount float64 `json:"total_amount"`
}

// ItemSummary 一条报销明细的汇总
type ItemSummary struct {
	Index       int              `json:"index"`
	Category    string           `json:"category"`
	Amount      float64          `json:"amount"`
	Description string           `json:"description"`
	Receipts    []ReceiptSummary `json:"receipts"`
}

// ReceiptSummary 单张票据的汇总
type ReceiptSummary struct {
	Index     int     `json:"index"`
	Category  string  `json:"category"`
	Amount    float64 `json:"amount"`
	Date      string  `json:"date"`
	ImagePath string  `json:"image_path"`
}

type ListInvoicesTool struct{ tools.Tool }

func NewListInvoicesTool(store infra.StateStore, logger *log.Logger) *ListInvoicesTool {
	t, err := tools.NewFunc[ListInvoicesInput, ListInvoicesOutput](
		ToolListInvoices,
		"列出当前会话的报销明细和票据汇总。已确认的明细按 Items 分组展示，未归类的票据列在 PendingReceipts 中。金额以人民币元为单位。",
		func(ctx context.Context, input ListInvoicesInput) (ListInvoicesOutput, error) {
			sid := getSessionID(ctx)

			var state types.ReimbursementState
			found, err := store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)
			if err != nil {
				logger.Error("读取票据状态失败", zap.String("会话ID", sid), zap.Error(err))
				return ListInvoicesOutput{}, fmt.Errorf("读取票据状态失败: %w", err)
			}

			if !found {
				return ListInvoicesOutput{}, nil
			}

			// 汇总已确认明细
			itemSummaries := make([]ItemSummary, 0, len(state.Items))
			totalReceipts := 0
			var totalAmount int64
			for i, item := range state.Items {
				rcpts := make([]ReceiptSummary, 0, len(item.Receipts))
				for j, rct := range item.Receipts {
					rcpts = append(rcpts, ReceiptSummary{
						Index:     j + 1,
						Category:  rct.Category,
						Amount:    float64(rct.Amount) / 100.0,
						Date:      rct.Date,
						ImagePath: rct.ImagePath,
					})
				}
				itemSummaries = append(itemSummaries, ItemSummary{
					Index:       i + 1,
					Category:    item.Category,
					Amount:      float64(item.Amount) / 100.0,
					Description: item.Description,
					Receipts:    rcpts,
				})
				totalReceipts += len(item.Receipts)
				totalAmount += item.Amount
			}

			// 汇总待归类票据
			pendingSummaries := make([]ReceiptSummary, 0, len(state.PendingReceipts))
			for i, rct := range state.PendingReceipts {
				pendingSummaries = append(pendingSummaries, ReceiptSummary{
					Index:     i + 1,
					Category:  rct.Category,
					Amount:    float64(rct.Amount) / 100.0,
					Date:      rct.Date,
					ImagePath: rct.ImagePath,
				})
			}

			totalCount := totalReceipts + len(state.PendingReceipts)
			logger.Info("列出票据汇总",
				zap.Int("明细数", len(itemSummaries)),
				zap.Int("待归类", len(pendingSummaries)),
				zap.Int("票据总数", totalCount))

			return ListInvoicesOutput{
				Items:           itemSummaries,
				PendingReceipts: pendingSummaries,
				TotalCount:      totalCount,
				TotalAmount:     float64(totalAmount) / 100.0,
			}, nil
		},
	)
	if err != nil {
		panic("创建列出票据汇总工具失败: " + err.Error())
	}
	logger.Debug("列出票据汇总工具初始化完成")
	return &ListInvoicesTool{t}
}
