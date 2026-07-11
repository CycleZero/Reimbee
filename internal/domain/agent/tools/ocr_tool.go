// Package tools 票据识别工具（OCR）
package tools

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

type OCRInput struct {
	ImagePath string `json:"image_path"`
}

type OCROutput struct {
	InvoiceCode   string  `json:"invoice_code"`
	InvoiceNumber string  `json:"invoice_number"`
	Amount        int64   `json:"amount"`
	Date          string  `json:"date"`
	SellerName    string  `json:"seller_name"`
	SellerTaxID   string  `json:"seller_tax_id"`
	BuyerName     string  `json:"buyer_name"`
	Category      string  `json:"category"`
	Confidence    float64 `json:"confidence"`
	RawText       string  `json:"raw_text"`
	Error         string  `json:"error,omitempty"`
	Retry         bool    `json:"retry"`
	Comment       string  `json:"comment"`
}

type OCRTool struct{ tools.Tool }

func NewOCRTool(recognizer infra.OCRRecognizer, storage infra.FileStorage, store infra.StateStore, receiptRepo *reimbursement.ReceiptRepo, logger *log.Logger) *OCRTool {
	t, err := tools.NewFunc[OCRInput, OCROutput](
		ToolRecognizeInvoice,
		"识别票据图片，自动提取金额、开票日期、费用类别、销售方等信息。识别失败时返回 error 字段，Agent 应引导用户手动输入。",
		func(ctx context.Context, input OCRInput) (OCROutput, error) {
			sid := getSessionID(ctx)
			logger.Debug("OCR工具开始识别票据", zap.String("图片路径", input.ImagePath), zap.String("sessionID", sid))

			// 读取图片
			reader, err := storage.Get(ctx, input.ImagePath)
			if err != nil {
				logger.Warn("读取票据图片失败", zap.Error(err))
				return OCROutput{Error: fmt.Sprintf("读取图片失败: %v", err), Retry: false}, nil
			}
			defer reader.Close()

			imageData, err := io.ReadAll(reader)
			if err != nil {
				logger.Warn("读取图片字节流失败", zap.Error(err))
				return OCROutput{Error: "读取图片数据失败", Retry: true}, nil
			}

			mimeType := inferMimeType(input.ImagePath)

			result, err := recognizer.Recognize(ctx, imageData, mimeType)
			if err != nil {
				logger.Warn("OCR识别失败", zap.Error(err))
				return OCROutput{Error: fmt.Sprintf("票据识别失败: %v", err), Retry: true}, nil
			}
			if result.Error != "" {
				logger.Warn("OCR返回错误", zap.String("错误", result.Error))
				return OCROutput{Error: result.Error, Retry: result.Retry}, nil
			}

			amountInCents := int64(result.Amount * 100)

			// 读取报销状态，防重复识别
			var state types.ReimbursementState
			store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)

			// 检查 PendingReceipts 中是否已有同一路径的票据
			for _, rct := range state.PendingReceipts {
				if rct.ImagePath == input.ImagePath {
					logger.Warn("跳过重复OCR", zap.String("路径", input.ImagePath))
					return OCROutput{Error: "该票据已识别过，请勿重复上传", Retry: false}, nil
				}
			}
			// 同时检查已归类的 Items 中是否已有
			for _, item := range state.Items {
				for _, rct := range item.Receipts {
					if rct.ImagePath == input.ImagePath {
						logger.Warn("跳过重复OCR（已在明细中）", zap.String("路径", input.ImagePath))
						return OCROutput{Error: "该票据已识别并归入明细，请勿重复上传", Retry: false}, nil
					}
				}
			}

			// OCR 结果存入 PendingReceipts，保留完整字段待用户归类
			pendingReceipt := types.ReceiptState{
				ImagePath:     input.ImagePath,
				Amount:        amountInCents,
				Category:      result.Category,
				Date:          result.Date,
				InvoiceCode:   result.InvoiceCode,
				InvoiceNo:     result.InvoiceNumber,
				SellerName:    result.SellerName,
				OCRRawAmount:  amountInCents,
				OCRConfidence: result.Confidence,
			}

			// 持久化票据到数据库（ItemID=0 表示尚未归类）
			receipt := &model.Receipt{
				ItemID:         0,
				InvoiceCode:    result.InvoiceCode,
				InvoiceNumber:  result.InvoiceNumber,
				Amount:         amountInCents,
				InvoiceDate:    result.Date,
				SellerName:     result.SellerName,
				Category:       result.Category,
				ImagePath:      input.ImagePath,
				OCRRawAmount:   amountInCents,
				OCRConfidence:  result.Confidence,
			}
			if err := receiptRepo.Create(receipt); err != nil {
				logger.Error("持久化票据到数据库失败", zap.Error(err))
				return OCROutput{Error: fmt.Sprintf("保存票据失败: %v", err), Retry: true}, nil
			}
			pendingReceipt.DBID = receipt.ID

			state.PendingReceipts = append(state.PendingReceipts, pendingReceipt)
			if state.CurrentPhase == "" {
				state.CurrentPhase = "phase1_collect"
			}
			store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)

			logger.Info("OCR识别成功", zap.Float64("金额(元)", result.Amount), zap.String("类别", result.Category))

		return OCROutput{
			InvoiceCode:   result.InvoiceCode,
			InvoiceNumber: result.InvoiceNumber,
			Amount:        amountInCents,
			Date:          result.Date,
			SellerName:    result.SellerName,
			SellerTaxID:   result.SellerTaxID,
			BuyerName:     result.BuyerName,
			Category:      result.Category,
			Confidence:    result.Confidence,
			RawText:       result.RawText,
			Comment:       "Amount金额单位为分，呈现时建议转换为元",
		}, nil
		},
	)
	if err != nil {
		panic("创建OCR工具失败: " + err.Error())
	}
	logger.Debug("OCR工具初始化完成")
	return &OCRTool{t}
}

func inferMimeType(path string) string {
	switch filepath.Ext(path) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".pdf":
		return "application/pdf"
	case ".bmp":
		return "image/bmp"
	case ".tiff", ".tif":
		return "image/tiff"
	default:
		return "image/jpeg"
	}
}
