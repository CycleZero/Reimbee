package infra

import "context"

// EmailSender 邮件发送器接口
type EmailSender interface {
	// SendReimbursementNotification 发送报销审批通知邮件
	// to: 收件人列表
	// subject: 邮件主题
	// body: HTML 邮件正文
	// attachment: 附件内容（PDF 字节）
	// filename: 附件文件名
	SendReimbursementNotification(ctx context.Context, to []string, subject, body string, attachment []byte, filename string) error
}
