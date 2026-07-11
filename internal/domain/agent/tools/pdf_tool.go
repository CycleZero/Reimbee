package tools

import (
	"bytes"
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// PDFInput generate_pdf 工具的输入参数
type PDFInput struct {
	ReimbursementID uint `json:"reimbursement_id"` // 报销单ID
}

// PDFOutput generate_pdf 工具的输出结果
type PDFOutput struct {
	PDFURL          string `json:"pdf_url"`          // PDF 预签名下载链接（MinIO，24小时有效）
	ReimbursementNo string `json:"reimbursement_no"` // 报销单号
}

// PDFTool Wire 命名类型（Blades tools.Tool）
type PDFTool struct{ tools.Tool }

// NewPDFTool 创建 PDF 生成工具，封装 infra.PDFGenerator + infra.FileStorage
func NewPDFTool(
	pdfGen infra.PDFGenerator,
	storage infra.FileStorage,
	reimbursementBiz *reimbursement.ReimbursementBiz,
	logger *log.Logger,
) *PDFTool {
	t, err := tools.NewFunc[PDFInput, PDFOutput](
		ToolGeneratePDF,
		"生成标准格式的报销单 PDF 文件并上传至对象存储。包含报销单号、申请人、票据明细、合规检查结果、审批人签名栏。返回一个 24 小时有效的预签名下载链接。",
		func(ctx context.Context, input PDFInput) (PDFOutput, error) {
			logger.Debug("PDF生成工具开始执行", zap.Uint("报销单ID", input.ReimbursementID))

			rm, err := reimbursementBiz.GetByID(input.ReimbursementID)
			if err != nil {
				logger.Error("查询报销单失败", zap.Uint("报销单ID", input.ReimbursementID), zap.Error(err))
				return PDFOutput{}, fmt.Errorf("报销单不存在: %w", err)
			}

			pdfBytes, err := pdfGen.GenerateReimbursementPDF(rm)
			if err != nil {
				logger.Error("生成PDF失败", zap.Uint("报销单ID", input.ReimbursementID), zap.Error(err))
				return PDFOutput{}, fmt.Errorf("PDF生成失败: %w", err)
			}

			pdfFileName := fmt.Sprintf("%s.pdf", rm.ReimbursementNo)
			uploaded, err := storage.Save(ctx, pdfFileName, "application/pdf", bytes.NewReader(pdfBytes))
			if err != nil {
				logger.Error("上传PDF到对象存储失败", zap.String("报销单号", rm.ReimbursementNo), zap.Error(err))
				return PDFOutput{}, fmt.Errorf("上传PDF失败: %w", err)
			}

			logger.Info("PDF生成并上传成功",
				zap.String("报销单号", rm.ReimbursementNo),
				zap.String("预签名URL", uploaded.URL),
				zap.Int("文件大小(bytes)", len(pdfBytes)),
			)

			return PDFOutput{
				PDFURL:          uploaded.URL,
				ReimbursementNo: rm.ReimbursementNo,
			}, nil
		},
	)
	if err != nil {
		panic("创建PDF生成工具失败: " + err.Error())
	}
	logger.Debug("PDF生成工具初始化完成")
	return &PDFTool{t}
}
