package infra

import "github.com/CycleZero/Reimbee/model"

// PDFGenerator PDF 生成器接口
type PDFGenerator interface {
	// GenerateReimbursementPDF 根据报销单生成 PDF 报销单文件，返回文件字节
	GenerateReimbursementPDF(rm *model.Reimbursement) ([]byte, error)
}
