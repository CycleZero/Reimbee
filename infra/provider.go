package infra

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewData,
	NewRedisClient,
	NewCustomRedisClient,
	GetDB,

	// OCR 识别器 —— 配置驱动工厂，根据 ocr.driver 决定实现
	NewOCRRecognizer,

	// PDF 生成器（默认 Mock）
	wire.Bind(new(PDFGenerator), new(*MockPDFGenerator)),
	NewMockPDFGenerator,

	// 邮件发送器（默认 Mock）
	wire.Bind(new(EmailSender), new(*MockEmailSender)),
	NewMockEmailSender,
)
