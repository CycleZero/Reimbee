package infra

import "context"

// MockOCRRecognizer 模拟 OCR 识别器，用于测试和演示
type MockOCRRecognizer struct {
	results map[string]*InvoiceResult // 图片哈希 → 识别结果
}

// NewMockOCRRecognizer 创建模拟 OCR 识别器
func NewMockOCRRecognizer() *MockOCRRecognizer {
	return &MockOCRRecognizer{results: make(map[string]*InvoiceResult)}
}

// Name 返回识别器名称
func (r *MockOCRRecognizer) Name() string { return "MockOCR" }

// HealthCheck 模拟永远健康
func (r *MockOCRRecognizer) HealthCheck(_ context.Context) error { return nil }

// Recognize 返回预置的模拟识别结果
func (r *MockOCRRecognizer) Recognize(_ context.Context, _ []byte, _ string) (*InvoiceResult, error) {
	return &InvoiceResult{
		InvoiceCode:   "044001900111",
		InvoiceNumber: "12345678",
		Amount:        1500.00,
		Date:          "2026-07-01",
		SellerName:    "XX科技有限公司",
		SellerTaxID:   "91440300MA5DXXXXX",
		BuyerName:     "中国石油大学（华东）",
		Category:      "办公用品",
		Confidence:    0.98,
	}, nil
}
