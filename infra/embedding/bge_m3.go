package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/viper"
	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
)

// BGEM3Embedder 通过 Ollama 本地部署的 BGE-M3 模型进行文本向量化
// 调用 Ollama POST /api/embed，BGE-M3 输出 1024 维向量
type BGEM3Embedder struct {
	endpoint string
	model    string
	client   *http.Client
	logger   *log.Logger
}

// NewBGEM3Embedder 创建 BGE-M3 向量化器实例
// 从 Viper 配置中读取 endpoint、model、timeout 等参数
func NewBGEM3Embedder(vc *viper.Viper, logger *log.Logger) *BGEM3Embedder {
	endpoint := vc.GetString("embedding.bge_m3.endpoint")
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	model := vc.GetString("embedding.bge_m3.model")
	if model == "" {
		model = "bge-m3"
	}
	timeout := vc.GetDuration("embedding.bge_m3.timeout")
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &BGEM3Embedder{
		endpoint: endpoint,
		model:    model,
		client:   &http.Client{Timeout: timeout},
		logger:   logger,
	}
}

// Dimensions 返回 BGE-M3 向量维度（1024）
func (e *BGEM3Embedder) Dimensions() int { return 1024 }

// ModelName 返回模型名称
func (e *BGEM3Embedder) ModelName() string { return e.model }

// HealthCheck 检查 Ollama 服务是否可用
func (e *BGEM3Embedder) HealthCheck(ctx context.Context) error {
	if e.endpoint == "" {
		return fmt.Errorf("BGE-M3 Ollama 端点未配置")
	}
	// 向 Ollama 发送一个轻量请求检查连通性
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.endpoint+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("构建健康检查请求失败: %w", err)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("Ollama 服务不可达: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama 服务异常 (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// Embed 调用 Ollama Embed API 将文本列表转换为向量
// 请求格式: POST {endpoint}/api/embed
// 请求体: {"model": "bge-m3", "input": [...], "truncate": true}
// 响应中 embeddings 为 [][]float32，需要转换为 [][]float64 返回
func (e *BGEM3Embedder) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("待向量化的文本列表不能为空")
	}

	e.logger.Debug("开始调用 BGE-M3 Embeddings API",
		zap.Int("文本数量", len(texts)),
		zap.String("模型", e.model),
		zap.String("端点", e.endpoint),
	)

	// 构建 Ollama Embed 请求体
	reqBody := map[string]any{
		"model":    e.model,
		"input":    texts,
		"truncate": true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		e.logger.Error("序列化 BGE-M3 请求体失败", zap.Error(err))
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	// 构建 HTTP 请求
	url := e.endpoint + "/api/embed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		e.logger.Error("构建 BGE-M3 请求失败", zap.Error(err))
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := e.client.Do(req)
	if err != nil {
		e.logger.Error("调用 BGE-M3 Embeddings API 失败", zap.Error(err))
		return nil, fmt.Errorf("调用 BGE-M3 Embeddings API 失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	// 处理非 200 响应
	if resp.StatusCode != http.StatusOK {
		e.logger.Warn("BGE-M3 Embeddings API 返回非 200 状态码",
			zap.Int("HTTP状态码", resp.StatusCode),
			zap.String("响应内容", string(respBytes)),
		)
		return nil, fmt.Errorf("BGE-M3 Embeddings API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	// 解析 Ollama Embed 响应 — embeddings 字段类型为 [][]float32
	var apiResp struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		e.logger.Error("解析 BGE-M3 响应失败", zap.Error(err))
		return nil, fmt.Errorf("解析 BGE-M3 响应失败: %w", err)
	}

	// 将 float32 向量转换为 float64
	vectors := make([][]float64, len(apiResp.Embeddings))
	for i, emb := range apiResp.Embeddings {
		vectors[i] = make([]float64, len(emb))
		for j, v := range emb {
			vectors[i][j] = float64(v)
		}
	}

	e.logger.Info("BGE-M3 Embeddings API 调用成功",
		zap.Int("输入文本数", len(texts)),
		zap.Int("返回向量数", len(vectors)),
	)

	return vectors, nil
}
