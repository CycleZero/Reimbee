package tools

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/tool/utils"
	"go.uber.org/zap"
)

// OCRInput recognize_invoice 工具的输入参数
type OCRInput struct {
	ImagePath string `json:"image_path" jsonschema:"required" jsonschema_description:"票据图片的存储路径（由上传接口返回）"`
}

// OCROutput recognize_invoice 工具的输出结果
type OCROutput struct {
	InvoiceCode string  `json:"invoice_code"` // 发票代码
	InvoiceNumber string `json:"invoice_number"` // 发票号码
	Amount      int64   `json:"amount"`       // 金额（分）
	Date        string  `json:"date"`         // 开票日期 YYYY-MM-DD
	SellerName  string  `json:"seller_name"`  // 销售方名称
	SellerTaxID string  `json:"seller_tax_id"` // 销售方税号
	BuyerName   string  `json:"buyer_name"`   // 购买方名称
	Category    string  `json:"category"`     // 费用类别
	Confidence  float64 `json:"confidence"`   // 识别置信度 0~1
	RawText     string  `json:"raw_text"`     // OCR 原始识别文本
	Error       string  `json:"error,omitempty"` // 错误信息（识别失败时填充）
	Retry       bool    `json:"retry"`        // 是否建议重试
}

// NewOCRTool 创建票据识别工具，封装 infra.OCRRecognizer + infra.FileStorage
// Phase 1（信息收集）阶段的核心加速工具——失败不阻塞流程，引导用户手动输入
func NewOCRTool(recognizer infra.OCRRecognizer, storage infra.FileStorage, logger *log.Logger) *OCRTool {
	t, err := utils.InferTool[OCRInput, OCROutput](
		"recognize_invoice",
		"识别票据图片，自动提取金额、开票日期、费用类别、销售方等信息。识别失败时返回 error 字段，Agent 应引导用户手动输入",
		func(ctx context.Context, input OCRInput) (OCROutput, error) {
			logger.Debug("OCR工具开始识别票据", zap.String("图片路径", input.ImagePath))

			reader, err := storage.Get(ctx, input.ImagePath)
			if err != nil {
				logger.Warn("读取票据图片失败", zap.String("路径", input.ImagePath), zap.Error(err))
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
				logger.Warn("OCR识别失败", zap.String("路径", input.ImagePath), zap.Error(err))
				return OCROutput{Error: fmt.Sprintf("票据识别失败: %v", err), Retry: true}, nil
			}

			if result.Error != "" {
				logger.Warn("OCR返回错误", zap.String("错误", result.Error), zap.Bool("可重试", result.Retry))
				return OCROutput{Error: result.Error, Retry: result.Retry}, nil
			}

			logger.Info("OCR识别成功",
				zap.Float64("金额(元)", result.Amount),
				zap.String("类别", result.Category),
				zap.Float64("置信度", result.Confidence),
			)

			amountInCents := int64(result.Amount * 100)

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
		panic("创建票据识别工具失败: " + err.Error())
	}
	logger.Debug("票据识别工具初始化完成")
	return &OCRTool{t}
}

// inferMimeType 根据文件扩展名推断 MIME 类型
func inferMimeType(path string) string {
	ext := filepath.Ext(path)
	switch ext {
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
		return "image/jpeg" // 默认 JPEG
	}
}
