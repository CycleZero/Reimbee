package infra

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewData,
	NewRedisClient,
	NewCustomRedisClient,
	GetDB,

	// OCR 识别器（默认使用 Mock 实现，生产通过配置切换）
	wire.Bind(new(OCRRecognizer), new(*MockOCRRecognizer)),
	NewMockOCRRecognizer,

	// PDF 生成器（默认 Mock）
	wire.Bind(new(PDFGenerator), new(*MockPDFGenerator)),
	NewMockPDFGenerator,

	// 邮件发送器（默认 Mock）
	wire.Bind(new(EmailSender), new(*MockEmailSender)),
	NewMockEmailSender,
)
