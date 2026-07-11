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

// EmailInput send_email 工具的输入参数
type EmailInput struct {
	ReimbursementID uint   `json:"reimbursement_id"` // 报销单ID
	PDFPath         string `json:"pdf_path"`          // PDF 文件路径（由 generate_pdf 工具返回）
}

// EmailOutput send_email 工具的输出结果
type EmailOutput struct {
	Success   bool     `json:"success"`
	Recipients []string `json:"recipients,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// EmailTool Wire 命名类型（Blades tools.Tool）
type EmailTool struct{ tools.Tool }

// NewEmailTool 创建邮件发送工具，封装 infra.EmailSender
func NewEmailTool(
	emailSender infra.EmailSender,
	reimbursementBiz *reimbursement.ReimbursementBiz,
	logger *log.Logger,
) *EmailTool {
	t, err := tools.NewFunc[EmailInput, EmailOutput](
		ToolSendEmail,
		"将生成的报销单 PDF 通过邮件发送给审批链中的所有审批人。收件人自动从审批记录中提取。",
		func(ctx context.Context, input EmailInput) (EmailOutput, error) {
			logger.Debug("邮件发送工具开始执行",
				zap.Uint("报销单ID", input.ReimbursementID),
				zap.String("PDF路径", input.PDFPath))

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
				logger.Warn("无有效收件人，跳过发送", zap.Uint("报销单ID", input.ReimbursementID))
				return EmailOutput{Success: false, Error: "无有效收件人（审批链中无邮箱地址）"}, nil
			}

			subject := fmt.Sprintf("【Reimbee】新的报销单 %s 待审批", rm.ReimbursementNo)
			body := fmt.Sprintf(`
				<h3>报销单待审批通知</h3>
				<p><b>报销单号：</b>%s</p>
				<p><b>申请人：</b>%s</p>
				<p><b>事由：</b>%s</p>
				<p>请登录 Reimbee 系统查看详情并完成审批。</p>
				<hr/>
				<p style="color:gray;font-size:12px">此邮件由 Reimbee 智能报销系统自动发送</p>
			`, rm.ReimbursementNo, rm.EmployeeName, rm.SubmitNote)

			err = emailSender.SendReimbursementNotification(
				ctx, recipients, subject, body, nil, fmt.Sprintf("%s.pdf", rm.ReimbursementNo),
			)
			if err != nil {
				logger.Warn("邮件发送失败（不阻塞流程）",
					zap.Strings("收件人", recipients), zap.Error(err))
				return EmailOutput{Success: false, Error: fmt.Sprintf("邮件发送失败: %v", err)}, nil
			}

			logger.Info("邮件发送成功",
				zap.Uint("报销单ID", input.ReimbursementID),
				zap.Strings("收件人", recipients))

			return EmailOutput{Success: true, Recipients: recipients}, nil
		},
	)
	if err != nil {
		panic("创建邮件发送工具失败: " + err.Error())
	}
	logger.Debug("邮件发送工具初始化完成")
	return &EmailTool{t}
}
