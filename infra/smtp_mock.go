package infra

import (
	"context"
	"fmt"
)

// MockEmailSender 模拟邮件发送器，用于测试和演示
type MockEmailSender struct {
	Sent []MockEmail // 记录发送的邮件
}

// MockEmail 模拟邮件记录
type MockEmail struct {
	To      []string
	Subject string
	Body    string
}

// NewMockEmailSender 创建模拟邮件发送器
func NewMockEmailSender() *MockEmailSender { return &MockEmailSender{} }

// SendReimbursementNotification 模拟发送邮件，记录到内存
func (s *MockEmailSender) SendReimbursementNotification(_ context.Context, to []string, subject, body string, _ []byte, _ string) error {
	s.Sent = append(s.Sent, MockEmail{To: to, Subject: subject, Body: body})
	return nil
}

// LastSent 返回最后一次模拟发送的邮件
func (s *MockEmailSender) LastSent() (*MockEmail, error) {
	if len(s.Sent) == 0 {
		return nil, fmt.Errorf("尚无模拟邮件发送记录")
	}
	return &s.Sent[len(s.Sent)-1], nil
}
