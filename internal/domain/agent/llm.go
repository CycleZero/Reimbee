package agent

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/CycleZero/Reimbee/log"
	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// ============================================
// ChatModel 配置驱动工厂
// ============================================

// NewChatModel 从 Viper 配置创建 OpenAI 兼容的 ChatModel 实例
// 复用 openai.base_url 和 openai.api_key，支持任意 OpenAI 兼容 API（如 deepseek、doubao）
func NewChatModel(vc *viper.Viper, logger *log.Logger) (model.ToolCallingChatModel, error) {
	apiKey := vc.GetString("openai.api_key")
	baseURL := vc.GetString("openai.base_url")
	chatModel := vc.GetString("openai.model")
	if chatModel == "" {
		chatModel = "gpt-4o"
	}

	if apiKey == "" {
		logger.Warn("OpenAI API Key 未配置，ChatModel 将无法正常工作")
	}

	temperature := float32(vc.GetFloat64("openai.temperature"))
	maxTokens := vc.GetInt("openai.max_tokens")

	logger.Debug("正在创建ChatModel实例",
		zap.String("模型", chatModel),
		zap.String("baseURL", baseURL),
		zap.Float32("temperature", temperature),
		zap.Int("maxTokens", maxTokens))

	config := &openaimodel.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   chatModel,
		Timeout: 30 * time.Second,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// 设置温度（如配置了）
	if temperature > 0 {
		config.Temperature = &temperature
	}

	// 设置最大 token 数（如配置了）
	if maxTokens > 0 {
		config.MaxTokens = &maxTokens
	}

	cm, err := openaimodel.NewChatModel(context.Background(), config)
	if err != nil {
		logger.Error("创建ChatModel失败", zap.Error(err))
		return nil, fmt.Errorf("创建ChatModel失败: %w", err)
	}

	logger.Info("ChatModel实例创建成功", zap.String("模型", chatModel))
	return cm, nil
}

// MustNewChatModel 创建 ChatModel，失败时 panic（匹配 Wire 模式）
func MustNewChatModel(vc *viper.Viper, logger *log.Logger) model.ToolCallingChatModel {
	cm, err := NewChatModel(vc, logger)
	if err != nil {
		panic("创建ChatModel失败: " + err.Error())
	}
	return cm
}
