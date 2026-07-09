package compliance

// ============================================
// 知识库管理 DTO
// ============================================

// PolicyDocumentResponse 文档列表项（不含分块内容）
type PolicyDocumentResponse struct {
	ID            uint   `json:"id"`
	Title         string `json:"title"`
	Version       string `json:"version"`
	EffectiveDate string `json:"effective_date"`
	Status        string `json:"status"`
	ChunkCount    int    `json:"chunk_count"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// PolicyDocumentDetailResponse 文档详情（含分块列表和全文）
type PolicyDocumentDetailResponse struct {
	ID            uint            `json:"id"`
	Title         string          `json:"title"`
	Content       string          `json:"content"`
	Version       string          `json:"version"`
	EffectiveDate string          `json:"effective_date"`
	Status        string          `json:"status"`
	ChunkCount    int             `json:"chunk_count"`
	Chunks        []ChunkResponse `json:"chunks"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

// ChunkResponse 分块响应
type ChunkResponse struct {
	ID         uint   `json:"id"`
	ChunkIndex int    `json:"chunk_index"`
	Content    string `json:"content"`
}

// UpdatePolicyRequest 更新文档请求
type UpdatePolicyRequest struct {
	Title         string `json:"title"`
	Content       string `json:"content"`
	Version       string `json:"version"`
	EffectiveDate string `json:"effective_date"`
	Status        string `json:"status"`
}

// KnowledgeBaseStatus 知识库运行状态
type KnowledgeBaseStatus struct {
	DocumentCount int64  `json:"document_count"`
	ChunkCount    int64  `json:"chunk_count"`
	SearchMode    string `json:"search_mode"`
	EmbedderModel string `json:"embedder_model,omitempty"`
	VectorStore   string `json:"vector_store,omitempty"`
	Healthy       bool   `json:"healthy"`
}

// SearchTestResponse 搜索测试响应
type SearchTestResponse struct {
	Query  string            `json:"query"`
	Mode   string            `json:"mode"`
	Chunks []SearchTestChunk `json:"chunks"`
}

// SearchTestChunk 搜索测试结果项（含来源文档信息）
type SearchTestChunk struct {
	DocumentID    uint    `json:"document_id"`
	DocumentTitle string  `json:"document_title"`
	ChunkIndex    int     `json:"chunk_index"`
	Content       string  `json:"content"`
	Score         float64 `json:"score,omitempty"`
}
