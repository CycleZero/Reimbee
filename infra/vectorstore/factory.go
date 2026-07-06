package vectorstore

import (
	"fmt"

	"github.com/CycleZero/Reimbee/log"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// NewVectorStore 根据配置创建对应的向量数据库实例
// 通过 config.yaml 中的 vector_store.driver 字段切换实现：
//   - "inmemory": 内存向量库（开发/测试，默认）
//   - "milvus":   Milvus 向量数据库（待实现）
//   - "pgvector": PostgreSQL pgvector 扩展（待实现）
//   - "chroma":   Chroma 向量数据库（待实现）
//
// dim 参数指定向量维度，用于创建集合/表时的维度校验。
func NewVectorStore(vc *viper.Viper, dim int, logger *log.Logger) (VectorStore, error) {
	driver := vc.GetString("vector_store.driver")
	if driver == "" {
		driver = "inmemory" // 默认使用内存实现，便于开发调试
	}

	logger.Debug("正在创建向量库实例",
		zap.String("驱动", driver),
		zap.Int("维度", dim))

	switch driver {
	case "inmemory":
		return NewInMemoryStore(dim), nil
	case "milvus":
		return NewMilvusStore(vc, dim, logger)
	case "pgvector":
		return NewPgvectorStore(vc, dim, logger)
	case "chroma":
		return NewChromaStore(vc, dim, logger)
	default:
		return nil, fmt.Errorf("未知向量库驱动: %s (可选: inmemory, milvus, pgvector, chroma)", driver)
	}
}
