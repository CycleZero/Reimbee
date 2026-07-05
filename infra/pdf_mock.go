package infra

import "github.com/CycleZero/Reimbee/model"

// MockPDFGenerator 模拟 PDF 生成器，用于测试和演示
type MockPDFGenerator struct{}

// NewMockPDFGenerator 创建模拟 PDF 生成器
func NewMockPDFGenerator() *MockPDFGenerator { return &MockPDFGenerator{} }

// GenerateReimbursementPDF 返回假 PDF 内容
func (g *MockPDFGenerator) GenerateReimbursementPDF(rm *model.Reimbursement) ([]byte, error) {
	return []byte("%% 模拟 PDF 报销单 " + rm.ReimbursementNo), nil
}
