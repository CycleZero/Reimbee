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

// ListInvoicesInput 列出票据汇总工具的输入参数（无需输入）
type ListInvoicesInput struct{}

// ListInvoicesOutput 列出票据汇总工具的输出结果
type ListInvoicesOutput struct {
	Count       int           `json:"count"`        // 票据数量
	TotalAmount float64       `json:"total_amount"` // 票据总金额（元）
	Invoices    []InvoiceItem `json:"invoices"`     // 票据明细列表
}

// InvoiceItem 单条票据明细
type InvoiceItem struct {
	Index     int     `json:"index"`      // 序号（从1开始）
	Category  string  `json:"category"`   // 费用类别
	Amount    float64 `json:"amount"`     // 金额（元）
	Date      string  `json:"date"`       // 开票日期
	ImagePath string  `json:"image_path"` // 票据图片路径
}

// ListInvoicesTool 列出票据汇总的共享工具（非中断式）
type ListInvoicesTool struct{ tools.Tool }

// NewListInvoicesTool 创建列出票据汇总工具，封装 infra.StateStore
func NewListInvoicesTool(store infra.StateStore, logger *log.Logger) *ListInvoicesTool {
	t, err := tools.NewFunc[ListInvoicesInput, ListInvoicesOutput](
		"list_invoices",
		"列出当前会话已收集的票据汇总，包括序号、费用类别、金额(元)、开票日期。金额均以人民币元为单位展示。",
		func(ctx context.Context, input ListInvoicesInput) (ListInvoicesOutput, error) {
			sid := getSessionID(ctx)

			var state types.ReimbursementState
			found, err := store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)
			if err != nil {
				logger.Error("读取票据状态失败", zap.String("会话ID", sid), zap.Error(err))
				return ListInvoicesOutput{}, fmt.Errorf("读取票据状态失败: %w", err)
			}

			if !found || len(state.Invoices) == 0 {
				logger.Info("列出票据汇总", zap.Int("数量", 0))
				return ListInvoicesOutput{
					Count:    0,
					Invoices: []InvoiceItem{},
				}, nil
			}

			items := make([]InvoiceItem, 0, len(state.Invoices))
			for i, inv := range state.Invoices {
				items = append(items, InvoiceItem{
					Index:     i + 1,
					Category:  inv.Category,
					Amount:    float64(inv.Amount) / 100.0, // 分转元
					Date:      inv.Date,
					ImagePath: inv.ImagePath,
				})
			}

			count := len(items)
			totalYuan := float64(state.TotalAmount) / 100.0 // 分转元

			logger.Info("列出票据汇总", zap.Int("数量", count))
			return ListInvoicesOutput{
				Count:       count,
				TotalAmount: totalYuan,
				Invoices:    items,
			}, nil
		},
	)
	if err != nil {
		panic("创建列出票据汇总工具失败: " + err.Error())
	}
	logger.Debug("列出票据汇总工具初始化完成")
	return &ListInvoicesTool{t}
}
