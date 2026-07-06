package infra

import (
	"github.com/CycleZero/Reimbee/infra/embedding"
	"github.com/CycleZero/Reimbee/infra/vectorstore"
	"github.com/CycleZero/Reimbee/log"
	"github.com/google/wire"
	"github.com/spf13/viper"
)

var ProviderSet = wire.NewSet(
	NewData,
	NewRedisClient,
	NewCustomRedisClient,
	GetDB,

	// OCR 识别器 —— 配置驱动工厂，根据 ocr.driver 决定实现
	MustNewOCRRecognizer,

	// 文件存储 —— 配置驱动工厂，根据 storage.driver 决定实现
	MustNewFileStorage,

	// PDF 生成器（真实实现，gofpdf 轻量引擎）
	NewGofpdfPDFGenerator,
	wire.Bind(new(PDFGenerator), new(*GofpdfPDFGenerator)),

	// 邮件发送器（真实 SMTP，配置驱动；未配置 SMTP 时降级 Mock）
	NewSMTPEmailSender,
	wire.Bind(new(EmailSender), new(*SMTPEmailSender)),

	// 会话持久化 —— MySQL 主存储 + Redis 缓存层
	NewMySQLSessionStore,
	wire.Bind(new(SessionStore), new(*MySQLSessionStore)),
	NewRedisSessionCache,

	// 嵌入模型 —— 配置驱动工厂
	MustNewEmbedder,

	// 向量库 —— 配置驱动工厂（依赖 Embedder 提供维度参数）
	MustNewVectorStore,
)

// MustNewEmbedder 包装 embedding.NewEmbedder，panic 替代 error（匹配项目规范）
func MustNewEmbedder(vc *viper.Viper, logger *log.Logger) embedding.Embedder {
	e, err := embedding.NewEmbedder(vc, logger)
	if err != nil {
		panic("创建嵌入模型失败: " + err.Error())
	}
	return e
}

// MustNewVectorStore 包装 vectorstore.NewVectorStore，从 Embedder 获取维度
func MustNewVectorStore(vc *viper.Viper, emb embedding.Embedder, logger *log.Logger) vectorstore.VectorStore {
	vs, err := vectorstore.NewVectorStore(vc, emb.Dimensions(), logger)
	if err != nil {
		panic("创建向量库失败: " + err.Error())
	}
	return vs
}

// MustNewOCRRecognizer 包装 NewOCRRecognizer，panic 替代 error
func MustNewOCRRecognizer(vc *viper.Viper) OCRRecognizer {
	r, err := NewOCRRecognizer(vc)
	if err != nil {
		panic("创建OCR识别器失败: " + err.Error())
	}
	return r
}

// MustNewFileStorage 包装 NewFileStorage，panic 替代 error
func MustNewFileStorage(vc *viper.Viper) FileStorage {
	s, err := NewFileStorage(vc)
	if err != nil {
		panic("创建文件存储失败: " + err.Error())
	}
	return s
}
