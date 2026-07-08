// Package agent LLM 模型工厂
package agent

import (
	"fmt"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades"
	"github.com/CycleZero/blades/contrib/openai"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// NewModel 从 Viper 配置创建 blades.ModelProvider
func NewModel(vc *viper.Viper, logger *log.Logger) (blades.ModelProvider, error) {
	apiKey := vc.GetString("openai.api_key")
	baseURL := vc.GetString("openai.base_url")
	chatModel := vc.GetString("openai.model")
	if chatModel == "" {
		chatModel = "gpt-4o"
	}

	temperature := vc.GetFloat64("openai.temperature")
	maxTokens := vc.GetInt64("openai.max_tokens")

	logger.Info("创建LLM模型实例",
		zap.String("模型", chatModel),
		zap.String("baseURL", baseURL),
		zap.Float64("temperature", temperature))

	cfg := openai.Config{
		APIKey:         apiKey,
		BaseURL:        baseURL,
		Temperature:    temperature,
		MaxOutputTokens: maxTokens,
	}

	model := openai.NewModel(chatModel, cfg)
	if model == nil {
		return nil, fmt.Errorf("创建LLM模型失败")
	}

	return model, nil
}

// MustNewModel 创建 blades.ModelProvider（Wire 兼容，失败 panic）
func MustNewModel(vc *viper.Viper, logger *log.Logger) blades.ModelProvider {
	model, err := NewModel(vc, logger)
	if err != nil {
		panic("创建LLM模型失败: " + err.Error())
	}
	return model
}
