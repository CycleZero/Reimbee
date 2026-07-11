package tools

import (
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

type EmailInput struct {
	ReimbursementID uint   `json:"reimbursement_id"`
	PDFPath         string `json:"pdf_path"`
}

type EmailOutput struct {
	Success    bool     `json:"success"`
	Recipients []string `json:"recipients,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type EmailTool struct{ tools.Tool }

func NewEmailTool(
	emailSender infra.EmailSender,
	store infra.StateStore,
	reimbursementBiz *reimbursement.ReimbursementBiz,
	logger *log.Logger,
) *EmailTool {
	t, err := tools.NewFunc[EmailInput, EmailOutput](
		ToolSendEmail,
		"将生成的报销单 PDF 通过邮件发送给审批链中的所有审批人，PDF 作为附件嵌入邮件。",
		func(ctx context.Context, input EmailInput) (EmailOutput, error) {
			rm, err := reimbursementBiz.GetByID(input.ReimbursementID)
			if err != nil {
				return EmailOutput{}, fmt.Errorf("查询报销单失败: %w", err)
			}

			recipients := make([]string, 0, len(rm.Approvals))
			for _, a := range rm.Approvals {
				if a.ApproverEmail != "" {
					recipients = append(recipients, a.ApproverEmail)
				}
			}
			if len(recipients) == 0 {
				return EmailOutput{Success: false, Error: "无有效收件人"}, nil
			}

			sid := getSessionID(ctx)
			var state types.ReimbursementState
			store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)

			var pdfBytes []byte
			if state.GeneratedPDF != "" {
				pdfBytes, _ = base64.StdEncoding.DecodeString(state.GeneratedPDF)
				state.GeneratedPDF = ""
				store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)
			}

			subject := fmt.Sprintf("【Reimbee】报销单 %s 待审批", rm.ReimbursementNo)
			body := fmt.Sprintf(`
				<h3>报销单待审批通知</h3>
				<p><b>报销单号：</b>%s</p>
				<p><b>申请人：</b>%s</p>
				<p><b>事由：</b>%s</p>
				<p>请登录 Reimbee 系统查看详情并完成审批。PDF 附件见本邮件。</p>
			`, rm.ReimbursementNo, rm.EmployeeName, rm.SubmitNote)

			err = emailSender.SendReimbursementNotification(
				ctx, recipients, subject, body, pdfBytes, fmt.Sprintf("%s.pdf", rm.ReimbursementNo),
			)
			if err != nil {
				logger.Warn("邮件发送失败", zap.Strings("收件人", recipients), zap.Error(err))
				return EmailOutput{Success: false, Error: fmt.Sprintf("邮件发送失败: %v", err)}, nil
			}

			logger.Info("邮件发送成功", zap.Strings("收件人", recipients))
			return EmailOutput{Success: true, Recipients: recipients}, nil
		},
	)
	if err != nil {
		panic("创建邮件发送工具失败: " + err.Error())
	}
	return &EmailTool{t}
}
