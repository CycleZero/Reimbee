package infra

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewData,
	NewRedisClient,
	NewCustomRedisClient,
	GetDB,

	// OCR 识别器 —— 配置驱动工厂，根据 ocr.driver 决定实现
	NewOCRRecognizer,

	// 文件存储 —— 配置驱动工厂，根据 storage.driver 决定实现
	NewFileStorage,

	// PDF 生成器（默认 Mock）
	wire.Bind(new(PDFGenerator), new(*MockPDFGenerator)),
	NewMockPDFGenerator,

	// 邮件发送器（默认 Mock）
	wire.Bind(new(EmailSender), new(*MockEmailSender)),
	NewMockEmailSender,

	// 会话持久化 —— MySQL 主存储 + Redis 缓存层
	NewMySQLSessionStore,
	wire.Bind(new(SessionStore), new(*MySQLSessionStore)),
	NewRedisSessionCache,
)
