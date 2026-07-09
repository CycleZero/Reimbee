// Package tools 智能体工具层
// create_reimbursement 工具：在数据库中创建报销单草稿（含明细和票据）
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

// CreateReimbInput 创建报销单工具的输入参数
type CreateReimbInput struct {
	EmployeeID   string              `json:"employee_id"`
	EmployeeName string              `json:"employee_name"`
	DepartmentID uint                `json:"department_id"`
	SubmitNote   string              `json:"submit_note"`
	Items        []CreateItemInput   `json:"items"` // 报销明细列表（含票据）
}

// CreateItemInput 创建报销单时的明细输入
type CreateItemInput struct {
	Category    string                `json:"category"`
	Amount      int64                 `json:"amount"`
	Description string                `json:"description"`
	Receipts    []CreateReceiptInput  `json:"receipts"`
}

// CreateReceiptInput 创建报销单时的票据输入
type CreateReceiptInput struct {
	ImagePath    string  `json:"image_path"`
	Amount       int64   `json:"amount"`
	Date         string  `json:"date"`
	InvoiceCode  string  `json:"invoice_code,omitempty"`
	InvoiceNo    string  `json:"invoice_no,omitempty"`
	SellerName   string  `json:"seller_name,omitempty"`
	OCRRawAmount int64   `json:"ocr_raw_amount,omitempty"`
	OCRConfidence float64 `json:"ocr_confidence,omitempty"`
}

// CreateReimbOutput 创建报销单工具的输出结果
type CreateReimbOutput struct {
	ReimbursementID uint   `json:"reimbursement_id"`
	ReimbursementNo string `json:"reimbursement_no"`
	Status          string `json:"status"`
}

type CreateReimbTool struct{ tools.Tool }

func NewCreateReimbTool(reimbursementBiz *reimbursement.ReimbursementBiz, store infra.StateStore, logger *log.Logger) *CreateReimbTool {
	t, err := tools.NewFunc[CreateReimbInput, CreateReimbOutput](
		"create_reimbursement",
		"在系统中创建报销单草稿（含报销明细和票据）。LLM 应先引导用户将OCR识别的票据归入明细，然后传入 items 参数。每个报销流程只需调用一次。",
		func(ctx context.Context, input CreateReimbInput) (CreateReimbOutput, error) {
			sid := getSessionID(ctx)
			logger.Debug("创建报销单工具开始执行",
				zap.String("工号", input.EmployeeID),
				zap.String("姓名", input.EmployeeName),
				zap.Uint("部门ID", input.DepartmentID),
				zap.Int("明细数", len(input.Items)))

			// 从 LLM 传入的 Items 构造 biz 层输入
			bizItems := make([]reimbursement.ItemInput, 0, len(input.Items))
			for _, item := range input.Items {
				receipts := make([]reimbursement.ReceiptInput, 0, len(item.Receipts))
				for _, rct := range item.Receipts {
					receipts = append(receipts, reimbursement.ReceiptInput{
						ImagePath:     rct.ImagePath,
						Amount:        rct.Amount,
						InvoiceDate:   rct.Date,
						InvoiceCode:   rct.InvoiceCode,
						InvoiceNumber: rct.InvoiceNo,
						SellerName:    rct.SellerName,
						OCRRawAmount:  rct.OCRRawAmount,
						OCRConfidence: rct.OCRConfidence,
					})
				}
				bizItems = append(bizItems, reimbursement.ItemInput{
					Category:    item.Category,
					Amount:      item.Amount,
					Description: item.Description,
					Receipts:    receipts,
				})
			}

			rm, err := reimbursementBiz.Create(&reimbursement.CreateReimbInput{
				EmployeeID:   input.EmployeeID,
				EmployeeName: input.EmployeeName,
				DepartmentID: input.DepartmentID,
				SubmitNote:   input.SubmitNote,
				Items:        bizItems,
			})
			if err != nil {
				logger.Error("创建报销单失败", zap.Error(err))
				return CreateReimbOutput{}, fmt.Errorf("创建报销单失败: %w", err)
			}

			// 更新 session state
			var state types.ReimbursementState
			store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)
			state.ReimbursementID = rm.ID
			store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)

			logger.Info("报销单创建成功",
				zap.Uint("ID", rm.ID),
				zap.String("单号", rm.ReimbursementNo),
				zap.String("状态", rm.Status))

			return CreateReimbOutput{
				ReimbursementID: rm.ID,
				ReimbursementNo: rm.ReimbursementNo,
				Status:          rm.Status,
			}, nil
		},
	)
	if err != nil {
		panic("创建reimbursement工具失败: " + err.Error())
	}
	logger.Debug("创建报销单工具初始化完成")
	return &CreateReimbTool{t}
}
