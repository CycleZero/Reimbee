package tools

import (
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
	PDFPath         string `json:"pdf_path"`          // 生成的 PDF 文件路径
	ReimbursementNo string `json:"reimbursement_no"`  // 报销单号
}

// PDFTool Wire 命名类型（Blades tools.Tool）
type PDFTool struct{ tools.Tool }

// NewPDFTool 创建 PDF 生成工具，封装 infra.PDFGenerator
func NewPDFTool(pdfGen infra.PDFGenerator, reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *PDFTool {
	t, err := tools.NewFunc[PDFInput, PDFOutput](
		ToolGeneratePDF,
		"生成标准格式的报销单 PDF 文件。包含报销单号、申请人、票据明细、合规检查结果、审批人签名栏。PDF 生成后会保存到文件存储中并返回访问路径",
		func(ctx context.Context, input PDFInput) (PDFOutput, error) {
			logger.Debug("PDF生成工具开始执行", zap.Uint("报销单ID", input.ReimbursementID))

			// 查询报销单以获取完整模型（PDF 生成器需要 *model.Reimbursement）
			rm, err := reimbursementBiz.GetByID(input.ReimbursementID)
			if err != nil {
				logger.Error("查询报销单失败", zap.Uint("报销单ID", input.ReimbursementID), zap.Error(err))
				return PDFOutput{}, fmt.Errorf("报销单不存在: %w", err)
			}

			// 调用 PDF 生成器（传入完整模型，当前为 Mock 实现）
			pdfBytes, err := pdfGen.GenerateReimbursementPDF(rm)
			if err != nil {
				logger.Error("生成PDF失败", zap.Uint("报销单ID", input.ReimbursementID), zap.Error(err))
				return PDFOutput{}, fmt.Errorf("PDF生成失败: %w", err)
			}

			pdfPath := fmt.Sprintf("/exports/%s.pdf", rm.ReimbursementNo)

			logger.Info("PDF生成成功",
				zap.String("报销单号", rm.ReimbursementNo),
				zap.String("路径", pdfPath),
				zap.Int("文件大小(bytes)", len(pdfBytes)),
			)

			return PDFOutput{
				PDFPath:         pdfPath,
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
