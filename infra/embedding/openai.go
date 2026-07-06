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

// OpenAIEmbedder 使用 OpenAI 兼容 Embeddings API 进行文本向量化
// 调用 POST {base_url}/embeddings，支持 text-embedding-3-small 等模型
type OpenAIEmbedder struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
	logger     *log.Logger
}

// NewOpenAIEmbedder 创建 OpenAI 向量化器实例
// 从 Viper 配置中读取 base_url、api_key、model、dimensions 等参数
func NewOpenAIEmbedder(vc *viper.Viper, logger *log.Logger) *OpenAIEmbedder {
	model := vc.GetString("embedding.openai.model")
	if model == "" {
		model = "text-embedding-3-small"
	}
	dimensions := vc.GetInt("embedding.openai.dimensions")
	if dimensions == 0 {
		dimensions = 512
	}
	return &OpenAIEmbedder{
		baseURL:    vc.GetString("openai.base_url"),
		apiKey:     vc.GetString("openai.api_key"),
		model:      model,
		dimensions: dimensions,
		client:     &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}

// Dimensions 返回向量维度
func (e *OpenAIEmbedder) Dimensions() int { return e.dimensions }

// ModelName 返回模型名称
func (e *OpenAIEmbedder) ModelName() string { return e.model }

// HealthCheck 检查 OpenAI Embeddings 服务是否可用
func (e *OpenAIEmbedder) HealthCheck(ctx context.Context) error {
	if e.apiKey == "" {
		return fmt.Errorf("OpenAI API Key 未配置")
	}
	if e.baseURL == "" {
		return fmt.Errorf("OpenAI Base URL 未配置")
	}
	return nil
}

// Embed 调用 OpenAI Embeddings API 将文本列表转换为向量
// 请求格式: POST {base_url}/embeddings
// 请求体: {"input": [...], "model": "...", "dimensions": N, "encoding_format": "float"}
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("待向量化的文本列表不能为空")
	}

	e.logger.Debug("开始调用 OpenAI Embeddings API",
		zap.Int("文本数量", len(texts)),
		zap.String("模型", e.model),
	)

	// 构建请求体
	reqBody := map[string]any{
		"input":           texts,
		"model":           e.model,
		"dimensions":      e.dimensions,
		"encoding_format": "float",
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		e.logger.Error("序列化 Embeddings 请求体失败", zap.Error(err))
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	// 构建 HTTP 请求
	url := e.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		e.logger.Error("构建 Embeddings 请求失败", zap.Error(err))
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	// 发送请求
	resp, err := e.client.Do(req)
	if err != nil {
		e.logger.Error("调用 Embeddings API 失败", zap.Error(err))
		return nil, fmt.Errorf("调用 Embeddings API 失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	// 处理非 200 响应
	if resp.StatusCode != http.StatusOK {
		e.logger.Warn("Embeddings API 返回非 200 状态码",
			zap.Int("HTTP状态码", resp.StatusCode),
			zap.String("响应内容", string(respBytes)),
		)
		return nil, fmt.Errorf("Embeddings API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	// 解析 OpenAI 格式响应
	var apiResp struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		e.logger.Error("解析 Embeddings 响应失败", zap.Error(err))
		return nil, fmt.Errorf("解析 Embeddings 响应失败: %w", err)
	}

	// 提取向量结果，按 index 排序
	vectors := make([][]float64, len(apiResp.Data))
	for _, item := range apiResp.Data {
		if item.Index < len(vectors) {
			vectors[item.Index] = item.Embedding
		}
	}

	e.logger.Info("OpenAI Embeddings API 调用成功",
		zap.Int("输入文本数", len(texts)),
		zap.Int("返回向量数", len(vectors)),
		zap.Int("Token用量", apiResp.Usage.TotalTokens),
	)

	return vectors, nil
}
