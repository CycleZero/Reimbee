package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

type PDFInput struct {
	ReimbursementID uint `json:"reimbursement_id"`
}

type PDFOutput struct {
	PDFURL          string `json:"pdf_url"`
	ReimbursementNo string `json:"reimbursement_no"`
}

type PDFTool struct{ tools.Tool }

func NewPDFTool(
	pdfGen infra.PDFGenerator,
	storage infra.FileStorage,
	store infra.StateStore,
	reimbursementBiz *reimbursement.ReimbursementBiz,
	logger *log.Logger,
) *PDFTool {
	t, err := tools.NewFunc[PDFInput, PDFOutput](
		ToolGeneratePDF,
		"生成标准格式的报销单 PDF 文件并上传至对象存储。返回预签名下载链接。PDF 同时存入会话状态供 send_email 附件使用。",
		func(ctx context.Context, input PDFInput) (PDFOutput, error) {
			logger.Debug("PDF生成工具开始执行", zap.Uint("报销单ID", input.ReimbursementID))

			rm, err := reimbursementBiz.GetByID(input.ReimbursementID)
			if err != nil {
				return PDFOutput{}, fmt.Errorf("报销单不存在: %w", err)
			}

			pdfBytes, err := pdfGen.GenerateReimbursementPDF(rm)
			if err != nil {
				return PDFOutput{}, fmt.Errorf("PDF生成失败: %w", err)
			}

			pdfFileName := fmt.Sprintf("%s.pdf", rm.ReimbursementNo)
			uploaded, err := storage.Save(ctx, pdfFileName, "application/pdf", bytes.NewReader(pdfBytes))
			if err != nil {
				return PDFOutput{}, fmt.Errorf("上传PDF失败: %w", err)
			}

			sid := getSessionID(ctx)
			var state types.ReimbursementState
			store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)
			state.GeneratedPDF = base64.StdEncoding.EncodeToString(pdfBytes)
			store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)

			logger.Info("PDF生成并上传成功",
				zap.String("报销单号", rm.ReimbursementNo),
				zap.String("预签名URL", uploaded.URL),
				zap.Int("文件大小(bytes)", len(pdfBytes)),
			)

			return PDFOutput{PDFURL: uploaded.URL, ReimbursementNo: rm.ReimbursementNo}, nil
		},
	)
	if err != nil {
		panic("创建PDF生成工具失败: " + err.Error())
	}
	return &PDFTool{t}
}
