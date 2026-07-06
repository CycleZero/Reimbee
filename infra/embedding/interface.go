// Package embedding 定义文本向量化能力抽象接口
// 通过 Embedder 接口统一不同的嵌入模型后端（OpenAI、BGE-M3、Qwen），
// 由配置驱动工厂在运行时选择合适的实现。
package embedding

import "context"

// Embedder 文本向量化接口
// 所有向量化实现必须满足此接口，通过配置驱动在运行时切换
type Embedder interface {
	// Embed 将一组文本转换为向量表示
	// texts: 待向量化的文本列表，每次调用建议不超过 100 条
	// 返回: 每个文本对应的向量（[][]float64），维度由 Dimensions() 定义
	Embed(ctx context.Context, texts []string) ([][]float64, error)

	// Dimensions 返回该模型生成的向量维度
	// 不同模型的维度不同：text-embedding-3-small 为 512，BGE-M3 为 1024，Qwen 为 1024
	Dimensions() int

	// ModelName 返回当前使用的模型名称，用于日志和诊断
	ModelName() string

	// HealthCheck 健康检查，判断嵌入服务是否可用
	HealthCheck(ctx context.Context) error
}
