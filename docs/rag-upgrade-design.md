# Reimbee RAG 检索升级方案 — 关键词匹配 → 向量嵌入语义搜索

> 版本: v1.0 | 日期: 2026-07-06 | 状态: 设计方案

---

## 一、设计目标

将 `internal/domain/compliance/knowledge.go` 中的**纯关键词匹配检索**升级为**基于 OpenAI 向量嵌入的语义搜索**，实现对政策文档的语义级理解（如"打车"能匹配"交通费"规则）。

### 当前 vs 升级后

| 维度 | 当前（关键词匹配） | 升级后（向量嵌入） |
|------|-------------------|-------------------|
| 检索算法 | `strings.Contains` 两层打分 | 余弦相似度 Top-K |
| 语义理解 | ❌ "打车" ≠ "交通" | ✅ 语义相近的文本自动匹配 |
| 新增依赖 | 无 | 无（复用现有 OpenAI 配置 + HTTP 客户端） |
| 新增基础设施 | 无 | 无（MySQL 存储向量 JSON，内存计算相似度） |
| 检索速度 | ~0.1ms（50 块） | ~0.5ms（50 块 × 512 维浮点运算） |
| 可审计性 | RuleID 追溯 | RuleID 追溯（不变） |

---

## 二、架构变更总览

```
                              升级前                          升级后
                              ──────                         ──────
索引阶段:
  PolicyDoc ──→ splitContent ──→ MySQL               PolicyDoc ──→ splitContent ──→ Embedding API ──→ MySQL
                 (纯文本分块)     (policy_chunks)                      (文本分块)     (生成向量)      (含 embedding 列)

检索阶段:
  "差旅-住宿 350元"                   "差旅-住宿 350元"
       │                                   │
       ▼                                   ▼
  strings.Contains loop              Embedding API → 查询向量
       │                                   │
       ▼                                   ▼
  Top-5 关键词命中                  余弦相似度 Top-5（语义匹配）

规则评估:
  extractRules → evaluate → pass/warning/error    （不变，完全复用）
```

### 新增/修改文件

| 文件 | 操作 | 说明 |
|------|:--:|------|
| `infra/embedding.go` | **新建** | OpenAI Embedding HTTP 客户端（复用 openai.base_url + api_key） |
| `infra/provider.go` | 修改 | Wire 注册 EmbeddingClient |
| `internal/domain/compliance/knowledge.go` | **重构** | Search() 替换为余弦相似度；IndexDocument() 新增向量生成 |
| `config.yaml` | 修改 | 新增 embedding_model、embedding_dimensions |
| `config.yaml.example` | 修改 | 同上 |

---

## 三、详细设计

### 3.1 嵌入模型选择

| 决策项 | 选择 | 理由 |
|--------|------|------|
| **模型** | `text-embedding-3-small` | ￥0.02/百万tokens，512维 RAG 场景足够 |
| **维度** | **512** | 存储缩小 3×（vs 1536），精度损失极小 |
| **批处理** | 单个索引时逐条调用；迁移时批量调用 | 简化实现 |
| **兼容性** | 通过 openai.base_url 支持任意 OpenAI 兼容 API | 深度求索/豆包等的 embedding API |

### 3.2 向量存储方案

**选择：MySQL 存储 JSON + 内存计算余弦相似度**

理由：
- 现有 `PolicyChunk.Embedding` 字段（`mediumtext`）已预留，直接使用
- 100-1000 块的规模，内存全量加载即可，无需外部向量数据库
- 512 维 × 1000 块 = 512,000 个 float64 ≈ 4MB 内存
- 查询延迟：1000 块 × 512 次浮点运算 ≈ 0.5ms

```
启动时:
  MySQL (policy_chunks.embedding) ──加载──→ knowledgeBase.chunks（含向量）
                                                   │
查询时:                                            │
  "交通费 150元" ──→ Embedding API ──→ queryVec    │
                                                   │
  queryVec 与 chunks[*].embedding 逐一计算余弦相似度  ←┘
                                                   │
  按相似度降序 → Top-5 → 进入规则评估
```

### 3.3 降级策略

| 场景 | 行为 |
|------|------|
| Embedding API 不可用 | 回退到**原有的关键词匹配**（保留 `searchByKeywords` 方法） |
| 块尚未生成向量（`embedding == nil`） | 跳过该块（仅搜索已有向量的块） |
| 向量维度不一致 | 跳过该块 + Warn 日志 |
| 知识库为空 | 返回 nil → 默认 pass |

### 3.4 新增配置

```yaml
# config.yaml — openai: 段新增
openai:
  base_url: "${OPENAI_BASE_URL}"
  api_key: "${OPENAI_API_KEY}"
  model: "${OPENAI_MODEL:-gpt-4o}"
  temperature: 0.3
  max_tokens: 4096
  embedding_model: "${EMBEDDING_MODEL:-text-embedding-3-small}"   # 新增
  embedding_dimensions: 512                                        # 新增
```

---

## 四、代码变更细节

### 4.1 `infra/embedding.go` — 新建

**设计原则**：与 `infra/ocr_multimodal.go` 风格完全一致——原生 `net/http`，无第三方 SDK。

```go
// EmbeddingClient OpenAI Embeddings HTTP 客户端
// 实现 embedding.Embedder 接口，供 Eino 组件直接使用
type EmbeddingClient struct {
    baseURL    string
    apiKey     string
    model      string
    dimensions int
    client     *http.Client
}

// NewEmbeddingClient 创建嵌入客户端（从 Viper 加载配置）
func NewEmbeddingClient(vc *viper.Viper) *EmbeddingClient

// EmbedStrings 批量生成文本嵌入向量
// 实现 Eino 的 embedding.Embedder 接口
func (c *EmbeddingClient) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error)

// Embed 单个文本嵌入（便捷方法）
func (c *EmbeddingClient) Embed(ctx context.Context, text string) ([]float64, error)
```

**关键实现细节**：
- 调用 `POST {base_url}/embeddings`
- 请求体：`{"input": texts, "model": "...", "dimensions": 512, "encoding_format": "float"}`
- 认证头：`Authorization: Bearer {api_key}`
- 超时：30 秒（与 OCR 客户端一致）
- 错误处理：HTTP 非 200 → 返回 error（由调用方决定降级策略）

### 4.2 `internal/domain/compliance/knowledge.go` — 重构

**核心变更**：

```go
// KnowledgeBase 新增 embeddingClient 字段
type KnowledgeBase struct {
    db              *gorm.DB
    chunks          []*model.PolicyChunk
    embeddingClient *EmbeddingClient  // 新增：向量生成
    mu              sync.RWMutex
}

// Search 重构——从关键词匹配改为余弦相似度
func (kb *KnowledgeBase) Search(ctx context.Context, query string, topK int) ([]*model.PolicyChunk, error) {
    // 1. 尝试向量搜索
    chunks, err := kb.searchByVector(ctx, query, topK)
    if err != nil {
        kb.logger.Warn("向量搜索失败，降级为关键词匹配", zap.Error(err))
        return kb.searchByKeywords(query, topK)  // 保留原方法作为降级
    }
    return chunks, nil
}

// searchByVector 基于余弦相似度的语义搜索
func (kb *KnowledgeBase) searchByVector(ctx context.Context, query string, topK int) ([]*model.PolicyChunk, error) {
    // 1. 生成查询向量
    queryVec, err := kb.embeddingClient.Embed(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("生成查询向量失败: %w", err)
    }

    // 2. 逐块计算余弦相似度
    type scored struct {
        chunk *model.PolicyChunk
        score float64
    }
    var results []scored

    for _, chunk := range kb.chunks {
        if chunk.Embedding == "" {
            continue  // 尚未生成向量的块跳过
        }
        chunkVec := parseEmbedding(chunk.Embedding)
        if len(chunkVec) != len(queryVec) {
            continue  // 维度不一致的块跳过（模型变更导致）
        }
        score := cosineSimilarity(queryVec, chunkVec)
        results = append(results, scored{chunk, score})
    }

    // 3. 排序取 Top-K
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

// IndexDocument 增强——存储前先生成向量
func (kb *KnowledgeBase) IndexDocument(ctx context.Context, doc *model.PolicyDocument) error {
    // ... 原有分块逻辑 ...

    for i, content := range chunks {
        // 新增：为每个分块生成向量嵌入
        vec, err := kb.embeddingClient.Embed(ctx, content)
        if err != nil {
            // 嵌入生成失败不阻塞索引，向量留空，后续可重试
            kb.logger.Warn("生成嵌入向量失败", zap.Int("分块", i), zap.Error(err))
        }
        embeddingJSON, _ := json.Marshal(vec)

        dbChunks = append(dbChunks, &model.PolicyChunk{
            DocumentID: doc.ID,
            ChunkIndex: i,
            Content:    content,
            Embedding:  string(embeddingJSON),  // 新增：存储向量
        })
    }
    // ...
}

// searchByKeywords 保留原关键词匹配方法作为降级方案
func (kb *KnowledgeBase) searchByKeywords(query string, topK int) []*model.PolicyChunk {
    // 原 Search 方法的核心逻辑（不变）
}

// cosineSimilarity 计算两个向量之间的余弦相似度
func cosineSimilarity(a, b []float64) float64 {
    var dot, normA, normB float64
    for i := range a {
        dot += a[i] * b[i]
        normA += a[i] * a[i]
        normB += b[i] * b[i]
    }
    if normA == 0 || normB == 0 {
        return 0
    }
    return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// parseEmbedding 从 JSON 字符串解析向量
func parseEmbedding(raw string) []float64 {
    var v []float64
    json.Unmarshal([]byte(raw), &v)
    return v
}
```

### 4.3 `infra/provider.go` — Wire 注册

```go
var ProviderSet = wire.NewSet(
    // ... 现有 Provider ...
    NewEmbeddingClient,   // 新增：OpenAI Embedding
    wire.Bind(new(embedding.Embedder), new(*EmbeddingClient)), // 绑定 Eino 接口
)
```

### 4.4 `config.yaml` + `config.yaml.example`

```yaml
openai:
  base_url: "${OPENAI_BASE_URL}"
  api_key: "${OPENAI_API_KEY}"
  model: "${OPENAI_MODEL:-gpt-4o}"
  temperature: 0.3
  max_tokens: 4096
  embedding_model: "${EMBEDDING_MODEL:-text-embedding-3-small}"  # 新增
  embedding_dimensions: 512                                       # 新增
```

---

## 五、数据迁移方案

从旧的关键词索引平滑迁移到向量索引：

```
1. 保留现有 policy_chunks 表（已有 embedding 列，允许 NULL）

2. 新增 API / CLI 命令：生成全量向量
   POST /api/admin/policies/reindex
   → 遍历所有 embedding IS NULL 的 chunk
   → 批量调用 Embedding API（每批 20 条，控制 rate limit）
   → 更新 embedding 列

3. 启动时自动检测：
   if 所有 chunk 的 embedding 均为 NULL:
       → 全量使用关键词匹配（向后兼容）
   else:
       → 优先使用向量搜索，失败降级关键词匹配

4. 灰度策略：
   - 初次部署：所有 chunk embedding = NULL → 关键词匹配
   - 执行 reindex → 向量填充完成
   - 下次查询自动切换到向量搜索
```

---

## 六、实施计划

| 步骤 | 内容 | 文件 | 预估时间 |
|:--:|------|------|:--:|
| 1 | 新建 `infra/embedding.go`（30秒超时 HTTP 客户端） | 新建 | 30min |
| 2 | 重构 `knowledge.go` 的 `Search()` 方法 | 修改 | 45min |
| 3 | 增强 `knowledge.go` 的 `IndexDocument()` 方法 | 修改 | 15min |
| 4 | 新增 `knowledge.go` 的 `cosineSimilarity()` 辅助函数 | 修改 | 10min |
| 5 | 新增 `knowledge.go` 的降级 `searchByKeywords()` 方法 | 修改 | 10min |
| 6 | Wire 注册 `EmbeddingClient` + 接口绑定 | 修改 | 10min |
| 7 | `config.yaml` 新增 embedding 配置项 | 修改 | 5min |
| 8 | 单元测试：余弦相似度计算（固定向量验证） | 测试 | 20min |
| 9 | 集成测试：Mock Embedding API + 端到端搜索 | 测试 | 30min |
| **合计** | | **5文件** | **~3h** |

---

## 七、风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| Embedding API 超时/限流 | 30s 超时 + 降级关键词匹配 |
| 模型变更导致向量维度不一致 | 启动时校验维度 + 跳过不一致的块 + Warn 日志 |
| 嵌入成本（首次全量索引） | ~100 chunks × 8191 tokens = 经济，约 $0.0001 |
| 向量搜索精度不如关键词（具体数字/金额） | 混合检索：向量 + 关键词双路召回，取并集 |
| MySQL embedding 列膨胀 | 单个 512 维向量 JSON ≈ 4KB，1000 块 ≈ 4MB，可忽略 |

---

*设计结束，待评审后实施。*
