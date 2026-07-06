package embedding

import (
	"fmt"

	"github.com/spf13/viper"
	"github.com/CycleZero/Reimbee/log"
)

// NewEmbedder 根据配置创建对应的文本向量化器实例
// 通过 config.yaml 中的 embedding.driver 字段切换实现:
//   - "openai":  OpenAI 兼容 Embeddings API（远程）
//   - "bge-m3":  BGE-M3 通过 Ollama 本地部署
//   - "qwen":    Qwen 模型（支持 Ollama 本地 / DashScope 云端双模式）
func NewEmbedder(vc *viper.Viper, logger *log.Logger) (Embedder, error) {
	driver := vc.GetString("embedding.driver")
	if driver == "" {
		driver = "openai" // 默认使用 OpenAI 嵌入
	}

	switch driver {
	case "openai":
		return NewOpenAIEmbedder(vc, logger), nil
	case "bge-m3":
		return NewBGEM3Embedder(vc, logger), nil
	case "qwen":
		return NewQwenEmbedder(vc, logger), nil
	default:
		return nil, fmt.Errorf("未知嵌入驱动: %s (可选: openai, bge-m3, qwen)", driver)
	}
}
