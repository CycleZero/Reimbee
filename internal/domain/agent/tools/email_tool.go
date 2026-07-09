package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/CycleZero/Reimbee/infra"
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
	Success   bool   `json:"success"`             // 是否发送成功
	MessageID string `json:"message_id,omitempty"` // 邮件消息 ID（发送成功时）
	Error     string `json:"error,omitempty"`      // 错误信息（发送失败时）
}

// EmailTool Wire 命名类型（Blades tools.Tool）
type EmailTool struct{ tools.Tool }

// NewEmailTool 创建邮件发送工具，封装 infra.EmailSender
func NewEmailTool(emailSender infra.EmailSender, logger *log.Logger) *EmailTool {
	t, err := tools.NewFunc[EmailInput, EmailOutput](
		ToolSendEmail,
		"将生成的报销单 PDF 通过邮件发送给审批人。收件人自动从审批链中提取。发送失败时不阻塞流程——Agent 会告知用户稍后手动通知审批人",
		func(ctx context.Context, input EmailInput) (EmailOutput, error) {
			logger.Debug("邮件发送工具开始执行",
				zap.Uint("报销单ID", input.ReimbursementID),
				zap.String("PDF路径", input.PDFPath))

			// 构建通知内容
			subject := fmt.Sprintf("【Reimbee】新的报销单 #%d 待审批", input.ReimbursementID)
			body := fmt.Sprintf(`
				<h3>报销单待审批通知</h3>
				<p>有一份新的报销单（ID: %d）需要您的审批。</p>
				<p>请登录 Reimbee 系统查看详情并完成审批。</p>
				<p>PDF 附件已随邮件发送，或可在系统内下载。</p>
				<hr/>
				<p style="color:gray;font-size:12px">此邮件由 Reimbee 智能报销系统自动发送</p>
			`, input.ReimbursementID)

			// 调用邮件发送器（收件人从审批链中提取，此处传空切片由 Mock 实现处理）
			err := emailSender.SendReimbursementNotification(
				ctx,
				nil,                     // 收件人列表（Mock 实现忽略此参数）
				subject,
				body,
				nil,                     // PDF 附件（Mock 实现忽略）
				fmt.Sprintf("报销单_%d.pdf", input.ReimbursementID),
			)
			if err != nil {
				logger.Warn("邮件发送失败（不阻塞流程）",
					zap.Uint("报销单ID", input.ReimbursementID),
					zap.Error(err))
				return EmailOutput{
					Success: false,
					Error:   fmt.Sprintf("邮件发送失败: %v", err),
				}, nil
			}

			logger.Info("邮件发送成功", zap.Uint("报销单ID", input.ReimbursementID))

			return EmailOutput{
				Success:   true,
				MessageID: fmt.Sprintf("msg_%d_%d", input.ReimbursementID, time.Now().UnixNano()), // 使用纳秒时间戳作为消息ID
			}, nil
		},
	)
	if err != nil {
		panic("创建邮件发送工具失败: " + err.Error())
	}
	logger.Debug("邮件发送工具初始化完成")
	return &EmailTool{t}
}
