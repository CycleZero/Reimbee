package compliance

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/infra/embedding"
	"github.com/CycleZero/Reimbee/infra/vectorstore"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// KnowledgeBase 合规知识库，管理政策文档的分块存储与检索
// v2.0：支持可插拔的向量嵌入模型 + 向量数据库后端，向量搜索失败自动降级为关键词匹配
type KnowledgeBase struct {
	db          *gorm.DB
	chunks      []*model.PolicyChunk
	embedder    embedding.Embedder       // 向量嵌入模型（可为 nil，nil 时仅关键词匹配）
	vectorStore vectorstore.VectorStore  // 向量数据库（可为 nil，nil 时仅关键词匹配）
	mu          sync.RWMutex
	logger      *log.Logger
}

// NewKnowledgeBase 创建知识库实例，自动迁移表结构并加载已有分块到内存
// embedder 和 vectorStore 为可选参数，传 nil 时仅使用关键词匹配（向后兼容）
func NewKnowledgeBase(data *infra.Data, embedder embedding.Embedder, vectorStore vectorstore.VectorStore, logger *log.Logger) *KnowledgeBase {
	if err := data.DB.AutoMigrate(&model.PolicyDocument{}, &model.PolicyChunk{}); err != nil {
		panic(fmt.Errorf("自动迁移政策表失败: %w", err))
	}

	kb := &KnowledgeBase{
		db:          data.DB,
		embedder:    embedder,
		vectorStore: vectorStore,
		logger:      logger,
	}
	kb.loadChunks()

	if embedder != nil && vectorStore != nil {
		logger.Info("知识库初始化完成（向量搜索模式）",
			zap.String("嵌入模型", embedder.ModelName()),
			zap.String("向量库", vectorStore.Name()),
			zap.Int("维度", embedder.Dimensions()))
	} else {
		logger.Info("知识库初始化完成（关键词匹配模式）")
	}
	return kb
}

// loadChunks 从数据库加载所有分块到内存缓存
func (kb *KnowledgeBase) loadChunks() {
	var chunks []*model.PolicyChunk
	if err := kb.db.Find(&chunks).Error; err != nil {
		return
	}
	kb.mu.Lock()
	kb.chunks = chunks
	kb.mu.Unlock()
}

// IndexDocument 将政策文档按段落分块后存入数据库和向量库
func (kb *KnowledgeBase) IndexDocument(ctx context.Context, doc *model.PolicyDocument) error {
	kb.logger.Debug("开始索引政策文档", zap.String("标题", doc.Title))

	if err := kb.db.WithContext(ctx).Create(doc).Error; err != nil {
		return fmt.Errorf("保存政策文档失败: %w", err)
	}

	chunks := splitContent(doc.Content, 500, 50)

	// 向量库存储（如有嵌入模型）
	var vectors []vectorstore.Vector
	if kb.embedder != nil && kb.vectorStore != nil {
		for i, content := range chunks {
			embeddings, err := kb.embedder.Embed(ctx, []string{content})
			if err != nil {
				kb.logger.Warn("生成嵌入向量失败，降级为关键词索引", zap.Int("分块", i), zap.Error(err))
				vectors = nil
				break
			}
			if len(embeddings) > 0 {
				vectors = append(vectors, vectorstore.Vector{
					ID:        fmt.Sprintf("policy-%d-%d", doc.ID, i),
					Content:   content,
					Embedding: embeddings[0],
					Metadata: map[string]string{
						"doc_id":      fmt.Sprintf("%d", doc.ID),
						"doc_title":   doc.Title,
						"version":     doc.Version,
						"chunk_index": fmt.Sprintf("%d", i),
					},
				})
			}
		}
	}

	// 写入向量库
	if len(vectors) > 0 {
		if err := kb.vectorStore.Store(ctx, vectors); err != nil {
			kb.logger.Warn("存储向量到向量库失败，关键词索引不受影响", zap.Error(err))
		} else {
			kb.logger.Info("向量索引完成", zap.Int("分块数", len(vectors)))
		}
	}

	// 写入数据库（MySQL 作为真实数据源，始终执行）
	var dbChunks []*model.PolicyChunk
	for i, content := range chunks {
		dbChunks = append(dbChunks, &model.PolicyChunk{
			DocumentID: doc.ID,
			ChunkIndex: i,
			Content:    content,
		})
	}

	if len(dbChunks) > 0 {
		if err := kb.db.WithContext(ctx).Create(&dbChunks).Error; err != nil {
			return fmt.Errorf("保存分块失败: %w", err)
		}
	}

	kb.mu.Lock()
	kb.chunks = append(kb.chunks, dbChunks...)
	kb.mu.Unlock()

	kb.logger.Info("政策文档索引完成",
		zap.String("标题", doc.Title),
		zap.Int("分块数", len(chunks)),
		zap.Int("向量数", len(vectors)))
	return nil
}

// scoredChunk 带相关性得分的分块
type scoredChunk struct {
	chunk *model.PolicyChunk
	score float64
}

// Search 检索最相关的 topK 个分块
// 优先使用向量语义搜索，失败或不可用时降级为关键词匹配
func (kb *KnowledgeBase) Search(ctx context.Context, query string, topK int) ([]*model.PolicyChunk, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	if len(kb.chunks) == 0 {
		return nil, nil
	}

	// 尝试向量搜索
	if kb.embedder != nil && kb.vectorStore != nil {
		chunks, err := kb.searchByVector(ctx, query, topK)
		if err == nil {
			return chunks, nil
		}
		kb.logger.Warn("向量搜索失败，降级为关键词匹配", zap.Error(err))
	}

	return kb.searchByKeywords(query, topK), nil
}

// searchByVector 基于向量的语义搜索
func (kb *KnowledgeBase) searchByVector(ctx context.Context, query string, topK int) ([]*model.PolicyChunk, error) {
	kb.logger.Debug("执行向量语义搜索", zap.String("查询", query))

	embeddings, err := kb.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("生成查询向量失败: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("嵌入模型返回空向量")
	}

	results, err := kb.vectorStore.Search(ctx, embeddings[0], topK, nil)
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}

	chunks := make([]*model.PolicyChunk, 0, len(results))
	for _, r := range results {
		chunk := &model.PolicyChunk{Content: r.Content}
		// 从向量库返回的 Metadata 中恢复 DocumentID 和 ChunkIndex
		if r.Metadata != nil {
			if docID, ok := r.Metadata["doc_id"]; ok {
				if id, err := strconv.Atoi(docID); err == nil {
					chunk.DocumentID = uint(id)
				}
			}
			if idx, ok := r.Metadata["chunk_index"]; ok {
				if i, err := strconv.Atoi(idx); err == nil {
					chunk.ChunkIndex = i
				}
			}
		}
		chunks = append(chunks, chunk)
	}

	kb.logger.Debug("向量搜索完成", zap.Int("结果数", len(chunks)))
	return chunks, nil
}

// searchByKeywords 基于关键词匹配的检索（降级方案）
func (kb *KnowledgeBase) searchByKeywords(query string, topK int) []*model.PolicyChunk {
	keywords := strings.Fields(query)
	var results []scoredChunk

	for _, chunk := range kb.chunks {
		score := 0.0
		lowerContent := chunk.Content

		for _, kw := range keywords {
			if strings.Contains(lowerContent, kw) {
				score += 3
			}
		}

		if strings.Contains(lowerContent, query) {
			score += 5
		}

		if score > 0 {
			results = append(results, scoredChunk{chunk: chunk, score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	out := make([]*model.PolicyChunk, len(results))
	for i, r := range results {
		out[i] = r.chunk
	}
	return out
}

// ReIndex 重建文档索引：删除旧分块和向量，重新分块和向量化
func (kb *KnowledgeBase) ReIndex(ctx context.Context, doc *model.PolicyDocument) error {
	// 删除旧分块
	if err := kb.db.Where("document_id = ?", doc.ID).Delete(&model.PolicyChunk{}).Error; err != nil {
		return fmt.Errorf("删除旧分块失败: %w", err)
	}

	// 删除旧向量
	if kb.vectorStore != nil {
		ids := make([]string, 0)
		for i := 0; ; i++ {
			ids = append(ids, fmt.Sprintf("policy-%d-%d", doc.ID, i))
			if i > 10000 {
				break // 安全上限
			}
		}
		if err := kb.vectorStore.Delete(ctx, ids); err != nil {
			kb.logger.Warn("删除旧向量失败，继续重建", zap.Error(err))
		}
	}

	// 重新分块
	chunks := splitContent(doc.Content, 500, 50)

	// 重新向量化
	var vectors []vectorstore.Vector
	if kb.embedder != nil && kb.vectorStore != nil {
		for i, content := range chunks {
			embeddings, err := kb.embedder.Embed(ctx, []string{content})
			if err != nil {
				kb.logger.Warn("重建索引时向量化失败", zap.Int("分块", i), zap.Error(err))
				vectors = nil
				break
			}
			if len(embeddings) > 0 {
				vectors = append(vectors, vectorstore.Vector{
					ID:        fmt.Sprintf("policy-%d-%d", doc.ID, i),
					Content:   content,
					Embedding: embeddings[0],
					Metadata: map[string]string{
						"doc_id":      fmt.Sprintf("%d", doc.ID),
						"doc_title":   doc.Title,
						"version":     doc.Version,
						"chunk_index": fmt.Sprintf("%d", i),
					},
				})
			}
		}
	}

	// 写入向量库
	if len(vectors) > 0 {
		if err := kb.vectorStore.Store(ctx, vectors); err != nil {
			kb.logger.Warn("重建索引时存储向量失败", zap.Error(err))
		}
	}

	// 写入数据库
	var dbChunks []*model.PolicyChunk
	for i, content := range chunks {
		dbChunks = append(dbChunks, &model.PolicyChunk{
			DocumentID: doc.ID,
			ChunkIndex: i,
			Content:    content,
		})
	}
	if len(dbChunks) > 0 {
		if err := kb.db.WithContext(ctx).Create(&dbChunks).Error; err != nil {
			return fmt.Errorf("保存新分块失败: %w", err)
		}
	}

	// 更新内存缓存
	kb.mu.Lock()
	var kept []*model.PolicyChunk
	for _, c := range kb.chunks {
		if c.DocumentID != doc.ID {
			kept = append(kept, c)
		}
	}
	kb.chunks = append(kept, dbChunks...)
	kb.mu.Unlock()

	kb.logger.Info("文档重建索引完成",
		zap.Uint("文档ID", doc.ID),
		zap.Int("分块数", len(chunks)),
		zap.Int("向量数", len(vectors)))
	return nil
}

// DeleteDocument 删除文档及其在向量库中的全部向量
func (kb *KnowledgeBase) DeleteDocument(ctx context.Context, docID uint) error {
	// 查询分块构建向量 ID 列表
	var chunks []*model.PolicyChunk
	if err := kb.db.Where("document_id = ?", docID).Find(&chunks).Error; err != nil {
		return fmt.Errorf("查询分块失败: %w", err)
	}

	// 删除向量库中的向量
	if kb.vectorStore != nil && len(chunks) > 0 {
		ids := make([]string, 0, len(chunks))
		for _, c := range chunks {
			ids = append(ids, fmt.Sprintf("policy-%d-%d", docID, c.ChunkIndex))
		}
		if err := kb.vectorStore.Delete(ctx, ids); err != nil {
			kb.logger.Warn("删除向量失败", zap.Error(err))
		}
	}

	// 删除文档（CASCADE 自动删关联 chunks）
	if err := kb.db.Delete(&model.PolicyDocument{}, docID).Error; err != nil {
		return fmt.Errorf("删除文档失败: %w", err)
	}

	// 更新内存缓存
	kb.mu.Lock()
	var kept []*model.PolicyChunk
	for _, c := range kb.chunks {
		if c.DocumentID != docID {
			kept = append(kept, c)
		}
	}
	kb.chunks = kept
	kb.mu.Unlock()

	kb.logger.Info("文档删除完成", zap.Uint("文档ID", docID))
	return nil
}

// Status 返回知识库运行状态
func (kb *KnowledgeBase) Status() KnowledgeBaseStatus {
	docCount, _ := kb.CountDocuments()
	chunkCount, _ := kb.CountChunks()

	mode := "keyword"
	embedderModel := ""
	vectorStoreName := ""
	healthy := true

	if kb.embedder != nil && kb.vectorStore != nil {
		mode = "vector"
		embedderModel = kb.embedder.ModelName()
		vectorStoreName = kb.vectorStore.Name()
		healthy = kb.vectorStore.HealthCheck(context.Background()) == nil
	}

	return KnowledgeBaseStatus{
		DocumentCount: docCount,
		ChunkCount:    chunkCount,
		SearchMode:    mode,
		EmbedderModel: embedderModel,
		VectorStore:   vectorStoreName,
		Healthy:       healthy,
	}
}

// GetDocumentTitle 根据文档 ID 查询标题（用于搜索测试展示来源）
func (kb *KnowledgeBase) GetDocumentTitle(docID uint) string {
	var doc model.PolicyDocument
	if err := kb.db.First(&doc, docID).Error; err != nil {
		return ""
	}
	return doc.Title
}

// CountDocuments 统计文档数
func (kb *KnowledgeBase) CountDocuments() (int64, error) {
	var count int64
	err := kb.db.Model(&model.PolicyDocument{}).Count(&count).Error
	return count, err
}

// CountChunks 统计分块数
func (kb *KnowledgeBase) CountChunks() (int64, error) {
	var count int64
	err := kb.db.Model(&model.PolicyChunk{}).Count(&count).Error
	return count, err
}

// splitContent 将文本按段落切分为固定大小的块，相邻块之间有重叠
// chunkSize: 每个块的目标字符数；overlap: 前一块末尾保留到下一块开头的字符数
func splitContent(content string, chunkSize, overlap int) []string {
	lines := strings.Split(content, "\n")
	var chunks []string
	var current strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if current.Len() > 0 && current.Len()+len(line) > chunkSize {
			chunks = append(chunks, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	if overlap > 0 && len(chunks) > 1 {
		for i := 1; i < len(chunks); i++ {
			prev := chunks[i-1]
			if len(prev) > overlap {
				// 从目标位置回退到有效 UTF-8 起始字节，防止切在中文中间
				start := len(prev) - overlap
				for start > 0 && (prev[start]&0xC0) == 0x80 {
					start--
				}
				overlapText := prev[start:]
				chunks[i] = overlapText + "\n" + chunks[i]
			}
		}
	}

	return chunks
}
