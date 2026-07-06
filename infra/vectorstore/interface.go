// Package vectorstore 提供向量数据库抽象层
// 支持内存（开发/测试）、Milvus、pgvector、Chroma 等多种后端，
// 通过配置驱动在运行时切换（见 factory.go）。
package vectorstore

import "context"

// Vector 向量记录，包含文本内容及其对应的嵌入向量
type Vector struct {
	ID        string            // 向量唯一标识
	Content   string            // 原始文本内容
	Embedding []float64         // 嵌入向量（定长浮点数组）
	Metadata  map[string]string // 附加元数据（如来源、时间戳等）
}

// SearchResult 向量相似度搜索结果
type SearchResult struct {
	ID       string            // 匹配向量的唯一标识
	Content  string            // 匹配向量的原始文本内容
	Score    float64           // 相似度分数（余弦相似度，范围 -1~1）
	Metadata map[string]string // 匹配向量的附加元数据
}

// VectorStore 向量数据库抽象接口
// 所有向量数据库后端必须实现此接口，通过配置驱动切换。
type VectorStore interface {
	// Store 批量存储向量记录，若某条记录的向量维度与集合不匹配则返回错误
	Store(ctx context.Context, vectors []Vector) error

	// Search 根据查询向量进行相似度搜索，返回 topK 个最相似的结果
	// filters 为可选的元数据过滤条件（键值对精确匹配），传 nil 或空 map 表示不过滤
	Search(ctx context.Context, query []float64, topK int, filters map[string]string) ([]SearchResult, error)

	// Delete 根据 ID 列表批量删除向量记录，不存在的 ID 静默忽略
	Delete(ctx context.Context, ids []string) error

	// Clear 清空当前集合/表中的所有向量记录
	Clear(ctx context.Context) error

	// Count 返回当前存储的向量记录总数
	Count(ctx context.Context) (int64, error)

	// Name 返回向量库名称，用于日志和诊断
	Name() string

	// HealthCheck 健康检查，判断向量数据库服务是否可用
	HealthCheck(ctx context.Context) error
}
