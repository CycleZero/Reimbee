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

// QwenEmbedder 使用 Qwen 模型进行文本向量化
// 支持两种部署模式：
//   - "ollama": 通过本地 Ollama 调用
//   - "dashscope": 通过阿里云 DashScope API 调用
type QwenEmbedder struct {
	mode   string // "ollama" 或 "dashscope"
	logger *log.Logger

	// Ollama 模式配置
	ollamaEndpoint string
	ollamaModel    string
	ollamaClient   *http.Client

	// DashScope 模式配置
	dashscopeAPIKey string
	dashscopeClient *http.Client
}

// NewQwenEmbedder 创建 Qwen 向量化器实例
// 根据 embedding.qwen.mode 配置决定使用 Ollama 本地模式还是 DashScope 云端 API 模式
func NewQwenEmbedder(vc *viper.Viper, logger *log.Logger) *QwenEmbedder {
	mode := vc.GetString("embedding.qwen.mode")
	if mode == "" {
		mode = "ollama"
	}

	e := &QwenEmbedder{
		mode:   mode,
		logger: logger,
	}

	// 初始化 Ollama 模式配置
	e.ollamaEndpoint = vc.GetString("embedding.qwen.ollama_endpoint")
	if e.ollamaEndpoint == "" {
		e.ollamaEndpoint = "http://localhost:11434"
	}
	e.ollamaModel = vc.GetString("embedding.qwen.ollama_model")
	if e.ollamaModel == "" {
		e.ollamaModel = "qwen3-embedding"
	}
	ollamaTimeout := vc.GetDuration("embedding.qwen.ollama_timeout")
	if ollamaTimeout == 0 {
		ollamaTimeout = 30 * time.Second
	}
	e.ollamaClient = &http.Client{Timeout: ollamaTimeout}

	// 初始化 DashScope 模式配置
	e.dashscopeAPIKey = vc.GetString("embedding.qwen.dashscope_api_key")
	dashscopeTimeout := vc.GetDuration("embedding.qwen.dashscope_timeout")
	if dashscopeTimeout == 0 {
		dashscopeTimeout = 30 * time.Second
	}
	e.dashscopeClient = &http.Client{Timeout: dashscopeTimeout}

	return e
}

// Dimensions 返回 Qwen 模型向量维度（1024）
func (e *QwenEmbedder) Dimensions() int { return 1024 }

// ModelName 返回当前使用的模型名称（含模式前缀）
func (e *QwenEmbedder) ModelName() string {
	switch e.mode {
	case "dashscope":
		return "text-embedding-v4"
	default:
		return e.ollamaModel
	}
}

// HealthCheck 检查 Qwen 嵌入服务是否可用
func (e *QwenEmbedder) HealthCheck(ctx context.Context) error {
	switch e.mode {
	case "dashscope":
		if e.dashscopeAPIKey == "" {
			return fmt.Errorf("Qwen DashScope API Key 未配置")
		}
		return nil
	default:
		if e.ollamaEndpoint == "" {
			return fmt.Errorf("Qwen Ollama 端点未配置")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.ollamaEndpoint+"/api/tags", nil)
		if err != nil {
			return fmt.Errorf("构建健康检查请求失败: %w", err)
		}
		resp, err := e.ollamaClient.Do(req)
		if err != nil {
			return fmt.Errorf("Ollama 服务不可达: %w", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Ollama 服务异常 (HTTP %d)", resp.StatusCode)
		}
		return nil
	}
}

// Embed 将文本列表转换为向量，根据 mode 分发到对应后端
func (e *QwenEmbedder) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("待向量化的文本列表不能为空")
	}

	switch e.mode {
	case "dashscope":
		return e.embedViaDashScope(ctx, texts)
	default:
		return e.embedViaOllama(ctx, texts)
	}
}

// embedViaOllama 通过本地 Ollama 调用 Qwen 模型进行向量化
func (e *QwenEmbedder) embedViaOllama(ctx context.Context, texts []string) ([][]float64, error) {
	e.logger.Debug("开始通过 Ollama 调用 Qwen Embeddings API",
		zap.Int("文本数量", len(texts)),
		zap.String("模型", e.ollamaModel),
		zap.String("端点", e.ollamaEndpoint),
	)

	// 构建 Ollama Embed 请求体
	reqBody := map[string]any{
		"model":    e.ollamaModel,
		"input":    texts,
		"truncate": true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		e.logger.Error("序列化 Qwen Ollama 请求体失败", zap.Error(err))
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.ollamaEndpoint+"/api/embed", bytes.NewReader(bodyBytes))
	if err != nil {
		e.logger.Error("构建 Qwen Ollama 请求失败", zap.Error(err))
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.ollamaClient.Do(req)
	if err != nil {
		e.logger.Error("调用 Qwen Ollama Embeddings API 失败", zap.Error(err))
		return nil, fmt.Errorf("调用 Qwen Ollama Embeddings API 失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.logger.Warn("Qwen Ollama Embeddings API 返回非 200 状态码",
			zap.Int("HTTP状态码", resp.StatusCode),
			zap.String("响应内容", string(respBytes)),
		)
		return nil, fmt.Errorf("Qwen Ollama Embeddings API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	// 解析 Ollama 响应
	var apiResp struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		e.logger.Error("解析 Qwen Ollama 响应失败", zap.Error(err))
		return nil, fmt.Errorf("解析 Qwen Ollama 响应失败: %w", err)
	}

	// float32 → float64
	vectors := make([][]float64, len(apiResp.Embeddings))
	for i, emb := range apiResp.Embeddings {
		vectors[i] = make([]float64, len(emb))
		for j, v := range emb {
			vectors[i][j] = float64(v)
		}
	}

	e.logger.Info("Qwen Ollama Embeddings API 调用成功",
		zap.Int("输入文本数", len(texts)),
		zap.Int("返回向量数", len(vectors)),
	)

	return vectors, nil
}

// embedViaDashScope 通过阿里云 DashScope API 调用 text-embedding-v4 模型
// API 文档: https://help.aliyun.com/zh/model-studio/text-embedding-api
func (e *QwenEmbedder) embedViaDashScope(ctx context.Context, texts []string) ([][]float64, error) {
	e.logger.Debug("开始调用 DashScope Text Embedding API",
		zap.Int("文本数量", len(texts)),
		zap.String("模型", "text-embedding-v4"),
	)

	// 构建 DashScope 请求体
	reqBody := map[string]any{
		"model": "text-embedding-v4",
		"input": map[string]any{
			"texts": texts,
		},
		"parameters": map[string]any{
			"dimension":   1024,
			"text_type":   "query",
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		e.logger.Error("序列化 DashScope 请求体失败", zap.Error(err))
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	// DashScope Text Embedding API 端点
	url := "https://dashscope-intl.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		e.logger.Error("构建 DashScope 请求失败", zap.Error(err))
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.dashscopeAPIKey)

	resp, err := e.dashscopeClient.Do(req)
	if err != nil {
		e.logger.Error("调用 DashScope Embeddings API 失败", zap.Error(err))
		return nil, fmt.Errorf("调用 DashScope Embeddings API 失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.logger.Warn("DashScope Embeddings API 返回非 200 状态码",
			zap.Int("HTTP状态码", resp.StatusCode),
			zap.String("响应内容", string(respBytes)),
		)
		return nil, fmt.Errorf("DashScope Embeddings API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	// 解析 DashScope 响应
	var apiResp struct {
		Output struct {
			Embeddings []struct {
				Embedding []float64 `json:"embedding"`
				TextIndex int       `json:"text_index"`
			} `json:"embeddings"`
		} `json:"output"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		e.logger.Error("解析 DashScope 响应失败", zap.Error(err))
		return nil, fmt.Errorf("解析 DashScope 响应失败: %w", err)
	}

	// 按 text_index 排序提取向量
	vectors := make([][]float64, len(apiResp.Output.Embeddings))
	for _, item := range apiResp.Output.Embeddings {
		if item.TextIndex < len(vectors) {
			vectors[item.TextIndex] = item.Embedding
		}
	}

	e.logger.Info("DashScope Embeddings API 调用成功",
		zap.Int("输入文本数", len(texts)),
		zap.Int("返回向量数", len(vectors)),
		zap.Int("Token用量", apiResp.Usage.TotalTokens),
	)

	return vectors, nil
}
