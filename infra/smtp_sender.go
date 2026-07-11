package infra

import (
	"bytes"
	"context"
	"fmt"
	"net/smtp"
	"strconv"

	"github.com/CycleZero/Reimbee/log"
	"github.com/jordan-wright/email"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// SMTPEmailSender 基于 SMTP 的真实邮件发送器
type SMTPEmailSender struct {
	config *SMTPConfig
	logger *log.Logger
}

// SMTPConfig SMTP 连接配置
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// NewSMTPEmailSender 创建 SMTP 邮件发送器
// 从 Viper 配置读取 SMTP 参数：smtp.host, smtp.port, smtp.user, smtp.password, smtp.from
func NewSMTPEmailSender(vc *viper.Viper, logger *log.Logger) *SMTPEmailSender {
	config := &SMTPConfig{
		Host:     vc.GetString("smtp.host"),
		Port:     vc.GetInt("smtp.port"),
		User:     vc.GetString("smtp.user"),
		Password: vc.GetString("smtp.password"),
		From:     vc.GetString("smtp.from"),
	}
	if config.From == "" {
		config.From = "noreply@reimbee.com"
	}

	logger.Debug("初始化SMTP邮件发送器",
		zap.String("host", config.Host),
		zap.Int("port", config.Port),
		zap.String("from", config.From))

	return &SMTPEmailSender{config: config, logger: logger}
}

// SendReimbursementNotification 发送报销审批通知邮件
func (s *SMTPEmailSender) SendReimbursementNotification(
	ctx context.Context, to []string, subject, body string, attachment []byte, filename string,
) error {
	s.logger.Debug("开始发送报销通知邮件",
		zap.Strings("收件人", to),
		zap.String("主题", subject))

	e := email.NewEmail()
	e.From = s.config.From
	e.To = to
	e.Subject = subject
	e.HTML = []byte(body)

	// 附件（如有）
	if len(attachment) > 0 && filename != "" {
		_, err := e.Attach(bytes.NewReader(attachment), filename, "application/pdf")
		if err != nil {
			s.logger.Error("添加附件失败", zap.String("文件名", filename), zap.Error(err))
			return fmt.Errorf("添加附件失败: %w", err)
		}
	}

	// SMTP 连接地址
	addr := s.config.Host + ":" + strconv.Itoa(s.config.Port)

	// 认证（如有用户名密码）
	var auth smtp.Auth
	if s.config.User != "" && s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.User, s.config.Password, s.config.Host)
	}

	// 发送
	err := e.Send(addr, auth)
	if err != nil {
		// 防御：to 可能为空，避免取 to[0] panic
		recipient := "(无收件人)"
		if len(to) > 0 {
			recipient = to[0]
		}
		s.logger.Error("邮件发送失败",
			zap.String("收件人", recipient),
			zap.String("主题", subject),
			zap.Error(err))
		return fmt.Errorf("邮件发送失败: %w", err)
	}

	s.logger.Info("邮件发送成功",
		zap.Strings("收件人", to),
		zap.String("主题", subject))
	return nil
}
