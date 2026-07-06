package vectorstore

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
)

// InMemoryStore 内存向量数据库，用于本地开发和单元测试
// 数据仅存在于进程内存中，重启后丢失。
type InMemoryStore struct {
	mu      sync.RWMutex
	vectors []Vector
	dim     int // 向量维度，用于校验输入
}

// NewInMemoryStore 创建内存向量数据库实例
// dim 为向量维度，所有存入的向量必须与此维度一致。
func NewInMemoryStore(dim int) *InMemoryStore {
	return &InMemoryStore{
		vectors: make([]Vector, 0),
		dim:     dim,
	}
}

// Name 返回向量库名称
func (s *InMemoryStore) Name() string { return "inmemory" }

// HealthCheck 内存向量库始终健康
func (s *InMemoryStore) HealthCheck(_ context.Context) error { return nil }

// Store 批量存储向量记录，校验向量维度一致性
func (s *InMemoryStore) Store(ctx context.Context, vectors []Vector) error {
	logger := log.GetLogger()

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, v := range vectors {
		if len(v.Embedding) != s.dim {
			return fmt.Errorf("向量维度不匹配：期望 %d，实际 %d（ID: %s）", s.dim, len(v.Embedding), v.ID)
		}
		s.vectors = append(s.vectors, v)
	}

	logger.Debug("向量已存储到内存库",
		zap.Int("新增数量", len(vectors)),
		zap.Int("当前总数", len(s.vectors)))
	return nil
}

// Search 基于余弦相似度的向量搜索，支持元数据过滤
func (s *InMemoryStore) Search(ctx context.Context, query []float64, topK int, filters map[string]string) ([]SearchResult, error) {
	logger := log.GetLogger()

	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(query) != s.dim {
		return nil, fmt.Errorf("查询向量维度不匹配：期望 %d，实际 %d", s.dim, len(query))
	}

	type scored struct {
		result SearchResult
		score  float64
	}

	var candidates []scored
	for _, v := range s.vectors {
		if !matchFilters(v.Metadata, filters) {
			continue
		}
		sim := cosineSimilarity(query, v.Embedding)
		candidates = append(candidates, scored{
			result: SearchResult{
				ID:       v.ID,
				Content:  v.Content,
				Score:    sim,
				Metadata: v.Metadata,
			},
			score: sim,
		})
	}

	// 按相似度降序排序（简单选择排序，测试/开发场景可接受）
	for i := 0; i < len(candidates); i++ {
		best := i
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[best].score {
				best = j
			}
		}
		if best != i {
			candidates[i], candidates[best] = candidates[best], candidates[i]
		}
	}

	// 截取 topK
	if topK > len(candidates) {
		topK = len(candidates)
	}
	results := make([]SearchResult, topK)
	for i := 0; i < topK; i++ {
		results[i] = candidates[i].result
	}

	logger.Debug("内存库相似度搜索完成",
		zap.Int("候选数量", len(candidates)),
		zap.Int("返回数量", topK))
	return results, nil
}

// Delete 根据 ID 删除向量记录，不存在的 ID 静默忽略
func (s *InMemoryStore) Delete(ctx context.Context, ids []string) error {
	logger := log.GetLogger()

	s.mu.Lock()
	defer s.mu.Unlock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	n := 0
	for _, v := range s.vectors {
		if !idSet[v.ID] {
			s.vectors[n] = v
			n++
		}
	}
	deleted := len(s.vectors) - n
	s.vectors = s.vectors[:n]

	logger.Debug("向量已从内存库删除",
		zap.Int("删除数量", deleted),
		zap.Int("当前总数", len(s.vectors)))
	return nil
}

// Clear 清空所有向量记录
func (s *InMemoryStore) Clear(ctx context.Context) error {
	logger := log.GetLogger()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.vectors = s.vectors[:0]
	logger.Debug("内存库向量已全部清空")
	return nil
}

// Count 返回当前向量记录总数
func (s *InMemoryStore) Count(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.vectors)), nil
}

// cosineSimilarity 计算两个向量的余弦相似度
// 公式：cos(θ) = (A·B) / (|A| × |B|)
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// matchFilters 检查元数据是否匹配所有过滤条件（键值对精确匹配）
// filters 为空时返回 true（不过滤）
func matchFilters(metadata, filters map[string]string) bool {
	if len(filters) == 0 {
		return true
	}
	for k, v := range filters {
		if mv, ok := metadata[k]; !ok || mv != v {
			return false
		}
	}
	return true
}
