// Package tools 票据识别工具（OCR）
package tools

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/log"
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
}

type OCRTool struct{ tools.Tool }

func NewOCRTool(recognizer infra.OCRRecognizer, storage infra.FileStorage, store infra.StateStore, logger *log.Logger) *OCRTool {
	t, err := tools.NewFunc[OCRInput, OCROutput](
		"recognize_invoice",
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

			// 读取报销状态，防重复 + 更新
			var state types.ReimbursementState
			store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)

			for _, inv := range state.Invoices {
				if inv.ImagePath == input.ImagePath {
					logger.Warn("跳过重复OCR", zap.String("路径", input.ImagePath))
					return OCROutput{Error: "该票据已识别过，请勿重复上传", Retry: false}, nil
				}
			}

		state.Invoices = append(state.Invoices, types.InvoiceState{
			ImagePath: input.ImagePath,
			Amount:    amountInCents,
			Category:  result.Category,
			Date:      result.Date,
		})
			state.TotalAmount += amountInCents
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
