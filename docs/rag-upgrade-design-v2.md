# Reimbee RAG 检索升级方案 v2 — 可插拔嵌入模型 + 向量库接口

> 版本: v2.0 | 日期: 2026-07-06 | 状态: 设计方案
> 替换 v1.0（单 OpenAI 方案），新增多模型、多向量库可插拔架构

---

## 一、设计原则

```
         ┌─────────────────────────────────────────────┐
         │              业务层（KnowledgeBase）          │
         │         不感知具体模型/向量库实现              │
         └──────────────┬──────────────┬────────────────┘
                        │              │
                ┌───────▼──────┐ ┌─────▼──────────┐
                │   Embedder   │ │  VectorStore    │
                │   (接口)     │ │   (接口)        │
                └───┬──┬──┬───┘ └─┬──┬──┬────────┘
                    │  │  │       │  │  │
        ┌───────────┘  │  └───────┘  │  └───────────┐
        ▼              ▼            ▼               ▼
   ┌─────────┐  ┌──────────┐  ┌─────────┐  ┌───────────┐
   │ OpenAI  │  │  BGE-M3  │  │ Milvus  │  │ pgvector  │  ...
   │ Emb     │  │  (Ollama)│  │         │  │           │
   └─────────┘  └──────────┘  └─────────┘  └───────────┘
```

**核心原则**：
- **接口隔离**：Embedder 和 VectorStore 各自独立的 Go 接口
- **配置驱动**：通过 config.yaml 选择具体实现，零代码切换
- **工厂模式**：`NewEmbedder(vc)` / `NewVectorStore(vc)` 根据配置返回对应实现
- **可组合**：任意 Embedder + 任意 VectorStore 可自由组合

---

## 二、接口设计

### 2.1 Embedder 接口

```go
// Embedder 文本向量化接口
// 所有嵌入模型实现必须满足此接口
type Embedder interface {
    // Embed 将文本批量转换为向量
    // texts: 待嵌入的文本列表（单次最多 batchSize 条）
    // 返回: 每个文本对应的向量，顺序与输入一致；单个向量长度 = Dimensions()
    Embed(ctx context.Context, texts []string) ([][]float64, error)

    // Dimensions 返回嵌入向量的固定维度
    // 索引和查询阶段必须使用相同维度的向量
    Dimensions() int

    // ModelName 返回模型标识（用于日志和审计）
    ModelName() string

    // HealthCheck 健康检查（判断模型服务是否可用）
    HealthCheck(ctx context.Context) error
}
```

### 2.2 VectorStore 接口

```go
// Vector 向量记录
type Vector struct {
    ID        string            // 唯一标识
    Content   string            // 原始文本（检索结果展示）
    Embedding []float64         // 向量数据
    Metadata  map[string]string // 额外元数据（文档ID、版本等）
}

// SearchResult 检索结果
type SearchResult struct {
    ID       string            // 向量ID
    Content  string            // 原始文本
    Score    float64           // 相似度分数（余弦相似度，[-1, 1]）
    Metadata map[string]string // 元数据
}

// VectorStore 向量存储与检索接口
type VectorStore interface {
    // Store 存储一批向量记录（创建或更新）
    Store(ctx context.Context, vectors []Vector) error

    // Search 检索与查询向量最相似的 Top-K 条记录
    Search(ctx context.Context, query []float64, topK int, filters map[string]string) ([]SearchResult, error)

    // Delete 按 ID 删除向量记录
    Delete(ctx context.Context, ids []string) error

    // Clear 清空当前集合/表的所有数据
    Clear(ctx context.Context) error

    // Count 返回已索引的向量数量
    Count(ctx context.Context) (int64, error)

    // Name 返回存储后端标识（用于日志）
    Name() string

    // HealthCheck 健康检查
    HealthCheck(ctx context.Context) error
}
```

---

## 三、Embedder 实现矩阵

| 实现 | 模型 | 维度 | 部署方式 | 标注 |
|------|------|:--:|------|------|
| `OpenAIEmbedder` | text-embedding-3-small | 512/1536 | OpenAI API / 兼容代理 | 复用 openai.base_url |
| `BGEM3Embedder` | BAAI/bge-m3 | **1024** | Ollama 本地 | 免费、多语言、8K tokens |
| `QwenEmbedder` | qwen2-embedding-7B / qwen3-embedding | **1536+** | Ollama 本地 / DashScope API | 阿里云 / 本地 |

### 3.1 OpenAIEmbedder

```
端点: {base_url}/embeddings
认证: Authorization: Bearer {api_key}
模型: text-embedding-3-small
维度: 512（通过 dimensions 参数控制）
```

**配置段**:
```yaml
embedding:
  driver: "openai"
  openai:
    model: "text-embedding-3-small"
    dimensions: 512
    # api_key 和 base_url 复用顶层 openai: 段
```

### 3.2 BGEM3Embedder（Ollama 本地部署）

**模型**: [BAAI/bge-m3](https://huggingface.co/BAAI/bge-m3)
- 维度: 1024
- 最大输入: 8192 tokens
- 语言: 100+ 种（中英文优秀）
- MTEB 评分: 多语言检索 SOTA 级别

**部署**:
```bash
ollama pull bge-m3
# 或使用 Xinference:
xinference launch --model-name bge-m3 --model-format pytorch
```

**API 端点**: `POST http://localhost:11434/api/embeddings`
```json
{
  "model": "bge-m3",
  "input": ["文本1", "文本2"]
}
```

**配置段**:
```yaml
embedding:
  driver: "bge-m3"
  bge_m3:
    endpoint: "http://localhost:11434"    # Ollama 地址
    model: "bge-m3"
    dimensions: 1024
    timeout: 30s
```

### 3.3 QwenEmbedder（Ollama / DashScope）

**Ollama 模式**（本地免费）:
```bash
ollama pull qwen2-embedding  # 或 qwen3-embedding
```

**DashScope 模式**（阿里云 API）:
```
端点: https://dashscope.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding
模型: text-embedding-v3
维度: 1024（默认）/ 768 / 512
```

**配置段**:
```yaml
embedding:
  driver: "qwen"
  qwen:
    # 模式选择
    mode: "ollama"               # ollama | dashscope
    # Ollama 配置
    ollama_endpoint: "http://localhost:11434"
    ollama_model: "qwen2-embedding"
    # DashScope 配置
    dashscope_api_key: "${DASHSCOPE_API_KEY}"
    dashscope_model: "text-embedding-v3"
    dimensions: 1024
    timeout: 30s
```

---

## 四、VectorStore 实现矩阵

| 实现 | 适用场景 | 部署复杂度 | 推荐度 |
|------|---------|:--:|:--:|
| `MilvusStore` | 千万级向量，高性能 ANN | 中（Docker 一键） | ⭐⭐⭐ 生产首选 |
| `PgvectorStore` | 已有 PostgreSQL，中度规模 | 低（SQL 扩展） | ⭐⭐ 运维简单 |
| `ChromaStore` | 轻量级，Python 生态 | 低（pip install） | ⭐ 原型验证 |

### 4.1 MilvusStore

**Go SDK**: `github.com/milvus-io/milvus-sdk-go/v2`

**部署**:
```bash
docker run -d --name milvus-standalone \
  -p 19530:19530 -p 9091:9091 \
  milvusdb/milvus:v2.5.x-latest
```

**配置段**:
```yaml
vector_store:
  driver: "milvus"
  milvus:
    endpoint: "localhost:19530"
    collection: "reimbee_policies"        # 集合名称
    dim: 1024                             # 嵌入维度（由 Embedder.Dimensions() 决定）
    metric_type: "COSINE"                 # L2 / COSINE / IP
    index_type: "IVF_FLAT"               # 索引类型
    nlist: 128                            # IVF 聚类数
    timeout: 30s
```

**关键操作映射**:

| 接口方法 | Milvus SDK 调用 |
|---------|----------------|
| `Store()` | `client.Insert()` |
| `Search()` | `client.Search()` with `entity.NewColumnFloatVector()` |
| `Delete()` | `client.DeleteByPks()` |
| `Clear()` | `client.DropCollection()` + recreate |
| `Count()` | `client.GetCollectionStatistics()` |

### 4.2 PgvectorStore

**Go 依赖**: `github.com/pgvector/pgvector-go` + `github.com/jackc/pgx/v5`

**创建表** (自动迁移):
```sql
CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE IF NOT EXISTS reimbee_policies (
    id        VARCHAR(128) PRIMARY KEY,
    content   TEXT NOT NULL,
    embedding vector(1024),
    metadata  JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_embedding
    ON reimbee_policies USING hnsw (embedding vector_cosine_ops);
```

**配置段**:
```yaml
vector_store:
  driver: "pgvector"
  pgvector:
    dsn: "postgres://user:pass@localhost:5432/reimbee?sslmode=disable"
    table: "reimbee_policies"
    dim: 1024
    index_type: "hnsw"                   # hnsw / ivfflat
```

**关键操作映射**:

| 接口方法 | pgvector SDK 调用 |
|---------|------------------|
| `Store()` | `INSERT ... ON CONFLICT (id) DO UPDATE` |
| `Search()` | `SELECT ... ORDER BY embedding <=> $1 LIMIT $2` |
| `Delete()` | `DELETE WHERE id = ANY($1)` |
| `Clear()` | `TRUNCATE TABLE` |
| `Count()` | `SELECT COUNT(*)` |

### 4.3 ChromaStore

Chroma 无官方 Go SDK，通过 **HTTP REST API** 交互。

**部署**:
```bash
docker run -d --name chroma -p 8000:8000 chromadb/chroma
```

**REST API 端点**:
```
POST   /api/v1/collections                        # 创建集合
POST   /api/v1/collections/{id}/add               # 添加向量
POST   /api/v1/collections/{id}/query              # 检索
DELETE /api/v1/collections/{id}                    # 删除集合
GET    /api/v1/collections/{id}                    # 获取集合信息
```

**配置段**:
```yaml
vector_store:
  driver: "chroma"
  chroma:
    endpoint: "http://localhost:8000"
    collection: "reimbee_policies"
    distance_metric: "cosine"            # cosine / l2 / ip
    timeout: 30s
```

**关键操作映射**:

| 接口方法 | Chroma HTTP 调用 |
|---------|-----------------|
| `Store()` | `POST /collections/{id}/add` with `{"embeddings": [...], "documents": [...], "ids": [...], "metadatas": [...]}` |
| `Search()` | `POST /collections/{id}/query` with `{"query_embeddings": [queryVec], "n_results": topK}` |
| `Delete()` | `POST /collections/{id}/delete` with `{"ids": [...]}` |
| `Clear()` | `DELETE /collections/{id}` |
| `Count()` | `GET /collections/{id}` → `resp["metadata"]["count"]` |

---

## 五、完整配置示例

```yaml
# ==========================================
# 嵌入模型配置
# ==========================================
embedding:
  driver: "bge-m3"                      # openai | bge-m3 | qwen

  openai:
    model: "text-embedding-3-small"
    dimensions: 512

  bge_m3:
    endpoint: "http://localhost:11434"
    model: "bge-m3"
    dimensions: 1024
    timeout: 30s

  qwen:
    mode: "ollama"                      # ollama | dashscope
    ollama_endpoint: "http://localhost:11434"
    ollama_model: "qwen2-embedding"
    dashscope_api_key: "${DASHSCOPE_API_KEY}"
    dashscope_model: "text-embedding-v3"
    dimensions: 1024
    timeout: 30s

# ==========================================
# 向量库配置
# ==========================================
vector_store:
  driver: "milvus"                      # milvus | pgvector | chroma

  milvus:
    endpoint: "localhost:19530"
    collection: "reimbee_policies"
    metric_type: "COSINE"
    index_type: "IVF_FLAT"
    nlist: 128
    timeout: 30s

  pgvector:
    dsn: "${PGVECTOR_DSN:-postgres://localhost:5432/reimbee}"
    table: "reimbee_policies"
    index_type: "hnsw"

  chroma:
    endpoint: "http://localhost:8000"
    collection: "reimbee_policies"
    distance_metric: "cosine"
    timeout: 30s
```

---

## 六、工厂模式实现

```go
// infra/embedding/factory.go

// NewEmbedder 配置驱动的嵌入器工厂
func NewEmbedder(vc *viper.Viper, logger *log.Logger) (Embedder, error) {
    driver := vc.GetString("embedding.driver")
    switch driver {
    case "openai":
        return NewOpenAIEmbedder(vc, logger)
    case "bge-m3":
        return NewBGEM3Embedder(vc, logger)
    case "qwen":
        return NewQwenEmbedder(vc, logger)
    default:
        return nil, fmt.Errorf("不支持的嵌入模型驱动: %s（可选: openai, bge-m3, qwen）", driver)
    }
}

// infra/vectorstore/factory.go

// NewVectorStore 配置驱动的向量库工厂
func NewVectorStore(vc *viper.Viper, dim int, logger *log.Logger) (VectorStore, error) {
    driver := vc.GetString("vector_store.driver")
    switch driver {
    case "milvus":
        return NewMilvusStore(vc, dim, logger)
    case "pgvector":
        return NewPgvectorStore(vc, dim, logger)
    case "chroma":
        return NewChromaStore(vc, dim, logger)
    default:
        return nil, fmt.Errorf("不支持的向量库驱动: %s（可选: milvus, pgvector, chroma）", driver)
    }
}
```

---

## 七、KnowledgeBase 集成

```go
// KnowledgeBase 重构后
type KnowledgeBase struct {
    embedder    Embedder       // 嵌入模型（可插拔）
    vectorStore VectorStore    // 向量库（可插拔）
    logger      *log.Logger
}

func NewKnowledgeBase(embedder Embedder, vectorStore VectorStore, logger *log.Logger) *KnowledgeBase {
    return &KnowledgeBase{
        embedder:    embedder,
        vectorStore: vectorStore,
        logger:      logger,
    }
}

// IndexDocuments 索引文档（分块 → 嵌入 → 存储）
func (kb *KnowledgeBase) IndexDocument(ctx context.Context, doc *model.PolicyDocument) error {
    chunks := splitContent(doc.Content, 500, 50)

    vectors := make([]Vector, 0, len(chunks))
    for i, content := range chunks {
        // 1. 生成向量
        embeddings, err := kb.embedder.Embed(ctx, []string{content})
        if err != nil {
            kb.logger.Warn("生成嵌入向量失败", zap.Int("分块", i), zap.Error(err))
            continue
        }

        vectors = append(vectors, Vector{
            ID:        fmt.Sprintf("%d-%d", doc.ID, i),
            Content:   content,
            Embedding: embeddings[0],
            Metadata: map[string]string{
                "doc_id":   strconv.Itoa(int(doc.ID)),
                "doc_title": doc.Title,
                "version":  doc.Version,
                "chunk_index": strconv.Itoa(i),
            },
        })
    }

    // 2. 批量存储到向量库
    return kb.vectorStore.Store(ctx, vectors)
}

// Search 语义搜索（嵌入查询 → 向量检索 → 返回分块）
func (kb *KnowledgeBase) Search(ctx context.Context, query string, topK int) ([]*model.PolicyChunk, error) {
    // 1. 生成查询向量
    embeddings, err := kb.embedder.Embed(ctx, []string{query})
    if err != nil {
        return nil, fmt.Errorf("生成查询向量失败: %w", err)
    }

    // 2. 向量相似度检索
    results, err := kb.vectorStore.Search(ctx, embeddings[0], topK, nil)
    if err != nil {
        return nil, fmt.Errorf("向量检索失败: %w", err)
    }

    // 3. 转换为 PolicyChunk
    chunks := make([]*model.PolicyChunk, 0, len(results))
    for _, r := range results {
        chunks = append(chunks, &model.PolicyChunk{
            Content: r.Content,
        })
    }
    return chunks, nil
}
```

---

## 八、文件结构

```
infra/
├── embedding/
│   ├── interface.go          # Embedder 接口定义
│   ├── factory.go            # 配置驱动工厂
│   ├── openai.go             # OpenAIEmbedder 实现
│   ├── bge_m3.go             # BGEM3Embedder（Ollama）实现
│   └── qwen.go               # QwenEmbedder（Ollama/DashScope）实现
├── vectorstore/
│   ├── interface.go          # VectorStore 接口 + Vector / SearchResult 类型
│   ├── factory.go            # 配置驱动工厂
│   ├── milvus.go             # MilvusStore 实现
│   ├── pgvector.go           # PgvectorStore 实现
│   └── chroma.go             # ChromaStore 实现（HTTP REST）
├── provider.go               # Wire 注册：Embedder + VectorStore 工厂
└── ...
```

---

## 九、Wire 依赖注入

```go
// infra/provider.go

var ProviderSet = wire.NewSet(
    // ... 现有 ...

    // 嵌入模型工厂
    embedding.NewEmbedder,
    wire.Bind(new(embedding.Embedder), new(*embedding.OpenAIEmbedder)),  // 默认绑定

    // 向量库工厂
    vectorstore.NewVectorStore,
    wire.Bind(new(vectorstore.VectorStore), new(*vectorstore.MilvusStore)), // 默认绑定
)
```

> 注：Wire 无法直接绑定接口到工厂返回的多种可能具体类型，需通过 `wire.Value` 或独立 ProviderSet 处理。实际实现时可能需要在 `wire.go` 中根据配置创建对应实例后注入。

---

## 十、实施优先级

| 优先级 | 内容 | 理由 |
|:--:|------|------|
| **P0** | `Embedder` 接口 + `VectorStore` 接口 | 基础设施，后续全部依赖 |
| **P0** | `OpenAIEmbedder`（已有配置） | 最快上线 |
| **P1** | `MilvusStore`（生产首选） | 高性能 ANN |
| **P1** | `KnowledgeBase` 重构 | 核心业务连接点 |
| **P2** | `BGEM3Embedder`（Ollama） | 免费本地模型 |
| **P2** | `PgvectorStore`（运维简单） | PostgreSQL 替代 |
| **P3** | `QwenEmbedder` | 阿里云生态 |
| **P3** | `ChromaStore` | 轻量级补充 |

---

## 附录 A：已验证的 API 规范

### A.1 BGE-M3 via Ollama — `/api/embed`

> ⚠️ 使用 `/api/embed`（不是 `/api/embeddings`）——后者已废弃，不支持批处理、不返回 L2 归一化向量。

| 属性 | 值 |
|------|-----|
| 端点 | `POST http://localhost:11434/api/embed` |
| 模型 | `bge-m3` |
| 维度 | **1024** |
| 最大 tokens | **8192** |
| 输出 | `[][]float32`（L2-归一化，单位向量） |
| 认证 | 无需（本地） |

**请求**:
```json
{"model": "bge-m3", "input": ["文本1", "文本2"], "truncate": true}
```

**Go 客户端参考**: Google oscar 项目 (`go.googlesource.com/oscar/internal/ollama`) 生产级实现

---

### A.2 Qwen via DashScope

| 属性 | 值 |
|------|-----|
| 端点 | `POST https://dashscope-intl.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding` |
| 模型 | `text-embedding-v4`（推荐）/ `text-embedding-v3` |
| 维度 | v4: 64~2048（默认1024）；v3: 512/768/1024 |
| 最大 tokens | 8192/文本 |
| 批大小 | 最多 10 条/请求 |
| 认证 | `Authorization: Bearer $DASHSCOPE_API_KEY` |

**Go 社区 SDK**: `github.com/casibase/dashscopego/embedding`
**OpenAI 兼容端点**: `POST {base_url}/compatible-mode/v1/embeddings`

---

### A.3 Milvus Go SDK v2

| 属性 | 值 |
|------|-----|
| **导入路径** | `github.com/milvus-io/milvus/client/v2`（旧 v2 SDK 已废弃） |
| 创建客户端 | `milvusclient.New(ctx, &ClientConfig{Address: "localhost:19530"})` |
| 创建集合 | `CreateCollection()` + schema + IndexOptions |
| 插入向量 | `Insert()` with `WithFloatVectorColumn("vector", dim, [][]float32)` |
| 搜索 | `Search()` with `NewSearchOption(collection, topK, []Vector).WithOutputFields("text")` |
| 删除 | `Delete()` with `WithInt64IDs()` 或 `WithExpr()` |
| 距离度量 | `entity.COSINE` / `entity.L2` / `entity.IP` |

---

### A.4 pgvector Go

| 属性 | 值 |
|------|-----|
| **导入路径** | `github.com/pgvector/pgvector-go` + `github.com/pgvector/pgvector-go/pgx` |
| 创建向量 | `pgvector.NewVector([]float32{...})` |
| 建表 | `CREATE TABLE docs (id BIGSERIAL, content TEXT, embedding vector(1024))` |
| 插入 | `INSERT ... VALUES ($1, pgvector.NewVector(emb))` |
| 搜索（余弦） | `SELECT * ORDER BY embedding <=> pgvector.NewVector(query) LIMIT N` |
| 索引 | `CREATE INDEX ON docs USING hnsw (embedding vector_cosine_ops)` |

**距离操作符**: `<=>` 余弦 / `<->` 欧几里得 / `<#>` 内积

---

### A.5 Chroma Go

| 属性 | 值 |
|------|-----|
| **Go SDK** | `github.com/amikos-tech/chroma-go/pkg/api/v2` |
| 创建客户端 | `chroma.NewHTTPClient(chroma.WithBaseURL("http://localhost:8000"))` |
| 创建集合 | `client.GetOrCreateCollection(ctx, name, WithHNSWSpaceCreate("cosine"))` |
| 添加向量 | `col.Add(ctx, WithIDs(...), WithEmbeddings(...), WithMetadatas(...))` |
| 搜索 | `col.Query(ctx, WithQueryEmbeddings(vec), WithNResults(5))` |
| 删除 | `col.Delete(ctx, WithIDs(...))` |

