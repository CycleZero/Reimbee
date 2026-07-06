package compliance

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"
	"gorm.io/gorm"
)

// KnowledgeBase 合规知识库，管理政策文档的分块存储与关键词检索
// 当前版本使用内存关键词匹配（后续可升级为向量嵌入 + 语义搜索）
type KnowledgeBase struct {
	db     *gorm.DB
	chunks []*model.PolicyChunk
	mu     sync.RWMutex
}

// NewKnowledgeBase 创建知识库实例，自动迁移表结构并加载已有分块到内存
func NewKnowledgeBase(data *infra.Data) *KnowledgeBase {
	if err := data.DB.AutoMigrate(&model.PolicyDocument{}, &model.PolicyChunk{}); err != nil {
		panic(fmt.Errorf("自动迁移政策表失败: %w", err))
	}

	kb := &KnowledgeBase{db: data.DB}
	kb.loadChunks()
	return kb
}

// loadChunks 从数据库加载所有分块到内存缓存
func (kb *KnowledgeBase) loadChunks() {
	var chunks []*model.PolicyChunk
	if err := kb.db.Find(&chunks).Error; err != nil {
		// 首次启动时表可能为空，不报错
		return
	}
	kb.mu.Lock()
	kb.chunks = chunks
	kb.mu.Unlock()
}

// IndexDocument 将政策文档按段落分块后存入数据库，并同步到内存索引
func (kb *KnowledgeBase) IndexDocument(ctx context.Context, doc *model.PolicyDocument) error {
	// 先保存文档主记录
	if err := kb.db.WithContext(ctx).Create(doc).Error; err != nil {
		return fmt.Errorf("保存政策文档失败: %w", err)
	}

	// 文本分块（chunkSize=500, overlap=50）
	chunks := splitContent(doc.Content, 500, 50)

	var dbChunks []*model.PolicyChunk
	for i, content := range chunks {
		dbChunks = append(dbChunks, &model.PolicyChunk{
			DocumentID: doc.ID,
			ChunkIndex: i,
			Content:    content,
		})
	}

	// 批量写入数据库
	if len(dbChunks) > 0 {
		if err := kb.db.WithContext(ctx).Create(&dbChunks).Error; err != nil {
			return fmt.Errorf("保存分块失败: %w", err)
		}
	}

	// 同步到内存索引
	kb.mu.Lock()
	kb.chunks = append(kb.chunks, dbChunks...)
	kb.mu.Unlock()

	return nil
}

// scoredChunk 带相关性得分的分块，用于排序
type scoredChunk struct {
	chunk *model.PolicyChunk
	score int
}

// Search 基于关键词在内存中检索最相关的 topK 个分块
func (kb *KnowledgeBase) Search(ctx context.Context, query string, topK int) ([]*model.PolicyChunk, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	if len(kb.chunks) == 0 {
		return nil, nil
	}

	// 分词：按空白字符拆分，同时保留原始 query 用于模糊匹配
	keywords := strings.Fields(query)
	var results []scoredChunk

	for _, chunk := range kb.chunks {
		score := 0
		lowerContent := chunk.Content

		for _, kw := range keywords {
			lowerKW := kw
			// 精确包含匹配
			if strings.Contains(lowerContent, lowerKW) {
				score += 3
			}
		}

		// 整句模糊匹配（高分加权）
		if strings.Contains(lowerContent, query) {
			score += 5
		}

		if score > 0 {
			results = append(results, scoredChunk{chunk: chunk, score: score})
		}
	}

	// 按相关性得分降序排列
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
	return out, nil
}

// splitContent 将文本按段落切分为固定大小的块，相邻块之间有重叠
// chunkSize: 每个块的目标字符数
// overlap: 前一块末尾保留到下一块开头的字符数
func splitContent(content string, chunkSize, overlap int) []string {
	lines := strings.Split(content, "\n")
	var chunks []string
	var current strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 当前块即将超出限制，先保存当前块
		if current.Len() > 0 && current.Len()+len(line) > chunkSize {
			chunks = append(chunks, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}

	// 保存最后一块
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	// 添加上下文重叠：将前一块末尾的 overlap 个字符拼接到下一块开头
	if overlap > 0 && len(chunks) > 1 {
		for i := 1; i < len(chunks); i++ {
			prev := chunks[i-1]
			if len(prev) > overlap {
				overlapText := prev[len(prev)-overlap:]
				chunks[i] = overlapText + "\n" + chunks[i]
			}
		}
	}

	return chunks
}
