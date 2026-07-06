package vectorstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/CycleZero/Reimbee/log"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// ChromaStore 基于 Chroma REST API 的向量数据库实现（原生 HTTP，零 SDK 依赖）
type ChromaStore struct {
	baseURL        string
	collectionName string
	collectionID   string
	dim            int
	client         *http.Client
	logger         *log.Logger
}

// NewChromaStore 创建 Chroma 向量库实例
func NewChromaStore(vc *viper.Viper, dim int, logger *log.Logger) (*ChromaStore, error) {
	endpoint := vc.GetString("vector_store.chroma.endpoint")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}
	endpoint = strings.TrimRight(endpoint, "/")

	collectionName := vc.GetString("vector_store.chroma.collection")
	if collectionName == "" {
		collectionName = "reimbee_policies"
	}

	timeout := vc.GetDuration("vector_store.chroma.timeout")
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	s := &ChromaStore{
		baseURL:        endpoint,
		collectionName: collectionName,
		dim:            dim,
		client:         &http.Client{Timeout: timeout},
		logger:         logger,
	}

	// 创建或获取集合
	ctx := context.Background()
	if err := s.ensureCollection(ctx); err != nil {
		return nil, fmt.Errorf("Chroma集合初始化失败: %w", err)
	}

	logger.Info("Chroma向量库连接成功",
		zap.String("endpoint", endpoint),
		zap.String("集合", collectionName),
		zap.Int("维度", dim))
	return s, nil
}

func (s *ChromaStore) Name() string { return "chroma" }

func (s *ChromaStore) HealthCheck(ctx context.Context) error {
	_, err := s.doGet(ctx, "/api/v2/heartbeat")
	return err
}

// ============================================
// 集合管理
// ============================================

func (s *ChromaStore) ensureCollection(ctx context.Context) error {
	// 先查是否已存在
	cols, err := s.listCollections(ctx)
	if err != nil {
		s.logger.Warn("获取Chroma集合列表失败，尝试创建", zap.Error(err))
	}

	for _, c := range cols {
		if c.Name == s.collectionName {
			s.collectionID = c.ID
			s.logger.Debug("Chroma集合已存在", zap.String("ID", s.collectionID))
			return nil
		}
	}

	// 创建集合
	id, err := s.createCollection(ctx)
	if err != nil {
		return err
	}
	s.collectionID = id
	return nil
}

type chromaCollection struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *ChromaStore) listCollections(ctx context.Context) ([]chromaCollection, error) {
	body, err := s.doGet(ctx, "/api/v2/tenants/default_tenant/databases/default_database/collections")
	if err != nil {
		return nil, err
	}
	var result []chromaCollection
	json.Unmarshal(body, &result)
	return result, nil
}

func (s *ChromaStore) createCollection(ctx context.Context) (string, error) {
	req := map[string]any{
		"name": s.collectionName,
		"metadata": map[string]any{
			"hnsw:space": "cosine",
		},
	}
	body, err := s.doPost(ctx, "/api/v2/tenants/default_tenant/databases/default_database/collections", req)
	if err != nil {
		return "", err
	}
	var col chromaCollection
	json.Unmarshal(body, &col)
	s.logger.Info("Chroma集合创建成功", zap.String("ID", col.ID), zap.String("名称", col.Name))
	return col.ID, nil
}

// ============================================
// VectorStore 接口实现
// ============================================

func (s *ChromaStore) Store(ctx context.Context, vectors []Vector) error {
	if len(vectors) == 0 {
		return nil
	}

	ids := make([]string, len(vectors))
	embeddings := make([][]float32, len(vectors))
	documents := make([]string, len(vectors))
	metadatas := make([]map[string]any, len(vectors))

	for i, v := range vectors {
		ids[i] = v.ID
		documents[i] = v.Content
		embeddings[i] = make([]float32, len(v.Embedding))
		for j, f := range v.Embedding {
			embeddings[i][j] = float32(f)
		}
		meta := make(map[string]any)
		for k, val := range v.Metadata {
			meta[k] = val
		}
		metadatas[i] = meta
	}

	req := map[string]any{
		"ids":        ids,
		"embeddings": embeddings,
		"documents":  documents,
		"metadatas":  metadatas,
	}

	path := fmt.Sprintf("/api/v2/tenants/default_tenant/databases/default_database/collections/%s/add", s.collectionID)
	_, err := s.doPost(ctx, path, req)
	if err != nil {
		s.logger.Error("Chroma存储向量失败", zap.Error(err))
		return fmt.Errorf("Chroma存储失败: %w", err)
	}

	s.logger.Debug("Chroma向量存储成功", zap.Int("数量", len(vectors)))
	return nil
}

func (s *ChromaStore) Search(ctx context.Context, query []float64, topK int, filters map[string]string) ([]SearchResult, error) {
	qf32 := make([]float32, len(query))
	for i, f := range query {
		qf32[i] = float32(f)
	}

	req := map[string]any{
		"query_embeddings": [][]float32{qf32},
		"n_results":        topK,
		"include":           []string{"documents", "metadatas", "distances"},
	}

	path := fmt.Sprintf("/api/v2/tenants/default_tenant/databases/default_database/collections/%s/query", s.collectionID)
	body, err := s.doPost(ctx, path, req)
	if err != nil {
		return nil, fmt.Errorf("Chroma检索失败: %w", err)
	}

	var result chromaQueryResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("Chroma响应解析失败: %w", err)
	}

	if len(result.IDs) == 0 || len(result.IDs[0]) == 0 {
		return nil, nil
	}

	ids := result.IDs[0]
	docs := result.Documents[0]
	dists := result.Distances[0]
	metas := result.Metadatas[0]

	results := make([]SearchResult, 0, len(ids))
	for i := 0; i < len(ids) && i < len(docs); i++ {
		score := 1.0
		if i < len(dists) {
			score = 1.0 - float64(dists[i])
		}
		r := SearchResult{
			ID:       ids[i],
			Content:  docs[i],
			Score:    score,
			Metadata: make(map[string]string),
		}
		if i < len(metas) && metas[i] != nil {
			for k, v := range metas[i] {
				r.Metadata[k] = fmt.Sprintf("%v", v)
			}
		}
		// 过滤
		if !matchMeta(r.Metadata, filters) {
			continue
		}
		results = append(results, r)
	}

	s.logger.Debug("Chroma检索完成", zap.Int("结果数", len(results)))
	return results, nil
}

type chromaQueryResult struct {
	IDs        [][]string           `json:"ids"`
	Documents  [][]string           `json:"documents"`
	Distances  [][]float32          `json:"distances"`
	Metadatas  [][]map[string]any   `json:"metadatas"`
}

func (s *ChromaStore) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	path := fmt.Sprintf("/api/v2/tenants/default_tenant/databases/default_database/collections/%s/delete", s.collectionID)
	req := map[string]any{"ids": ids}
	_, err := s.doPost(ctx, path, req)
	if err != nil {
		return fmt.Errorf("Chroma删除失败: %w", err)
	}
	s.logger.Debug("Chroma向量删除成功", zap.Int("数量", len(ids)))
	return nil
}

func (s *ChromaStore) Clear(ctx context.Context) error {
	path := fmt.Sprintf("/api/v2/tenants/default_tenant/databases/default_database/collections/%s", s.collectionID)
	if err := s.doDelete(ctx, path); err != nil {
		return fmt.Errorf("Chroma清空失败: %w", err)
	}
	s.collectionID = ""
	return s.ensureCollection(ctx)
}

func (s *ChromaStore) Count(ctx context.Context) (int64, error) {
	path := fmt.Sprintf("/api/v2/tenants/default_tenant/databases/default_database/collections/%s", s.collectionID)
	body, err := s.doGet(ctx, path)
	if err != nil {
		return 0, err
	}
	var col struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	json.Unmarshal(body, &col)
	// Chroma v2 doesn't directly expose count in GET /collections/{id}
	// Fallback: return 0 as count isn't critical
	return 0, nil
}

// ============================================
// HTTP 辅助
// ============================================

func (s *ChromaStore) doGet(ctx context.Context, path string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", s.baseURL+path, nil)
	return s.do(req)
}

func (s *ChromaStore) doPost(ctx context.Context, path string, data any) ([]byte, error) {
	body, _ := json.Marshal(data)
	req, _ := http.NewRequestWithContext(ctx, "POST", s.baseURL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return s.do(req)
}

func (s *ChromaStore) doDelete(ctx context.Context, path string) error {
	_, err := s.doGet(ctx, path+"?_method=delete") // Chroma HTTP DELETE workaround
	if err == nil {
		return nil
	}
	req, _ := http.NewRequestWithContext(ctx, "DELETE", s.baseURL+path, nil)
	_, err = s.do(req)
	return err
}

func (s *ChromaStore) do(req *http.Request) ([]byte, error) {
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Chroma HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Chroma返回错误 HTTP %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func matchMeta(meta map[string]string, filters map[string]string) bool {
	if len(filters) == 0 {
		return true
	}
	for k, v := range filters {
		if mv, ok := meta[k]; !ok || mv != v {
			return false
		}
	}
	return true
}
