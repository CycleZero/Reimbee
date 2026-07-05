// Package infra 定义 OCR 票据识别能力抽象接口
package infra

import "context"

// InvoiceResult OCR 识别后返回的结构化票据信息
type InvoiceResult struct {
	InvoiceCode   string  `json:"invoice_code"`   // 发票代码
	InvoiceNumber string  `json:"invoice_number"` // 发票号码
	Amount        float64 `json:"amount"`         // 金额（元）
	Date          string  `json:"date"`           // 开票日期 YYYY-MM-DD
	SellerName    string  `json:"seller_name"`    // 销售方名称
	SellerTaxID   string  `json:"seller_tax_id"`  // 销售方税号
	BuyerName     string  `json:"buyer_name"`     // 购买方名称
	Category      string  `json:"category"`       // 费用类别（OCR 推断）
	Confidence    float64 `json:"confidence"`     // 识别置信度 0~1
	RawText       string  `json:"raw_text"`       // OCR 原始识别文本
	Error         string  `json:"error,omitempty"` // 错误信息（识别失败时填充）
	Retry         bool    `json:"retry"`          // 是否建议重试
}

// OCRRecognizer OCR 识别器接口
// 所有 OCR 实现必须满足此接口，通过配置驱动在运行时切换
type OCRRecognizer interface {
	// Recognize 识别单张票据图片，返回结构化票据信息
	Recognize(ctx context.Context, imageData []byte, mimeType string) (*InvoiceResult, error)

	// Name 返回识别器名称，用于日志和诊断
	Name() string

	// HealthCheck 健康检查，判断 OCR 服务是否可用
	HealthCheck(ctx context.Context) error
}
