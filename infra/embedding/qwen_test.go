package embedding

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
)

func newTestLogger() *log.Logger {
	return &log.Logger{Logger: zap.NewNop()}
}

func TestQwenEmbedder_DashScope_InvalidAPIKey(t *testing.T) {
	vc := viper.New()
	vc.Set("embedding.qwen.mode", "dashscope")
	vc.Set("embedding.qwen.dashscope_api_key", "sk-invalid-test-key")

	e := NewQwenEmbedder(vc, newTestLogger())

	if e.ModelName() != "text-embedding-v4" {
		t.Fatalf("期望 model=text-embedding-v4, 实际=%s", e.ModelName())
	}

	vectors, err := e.Embed(context.Background(), []string{"测试文本"})
	if err == nil {
		t.Logf("嵌入成功(API Key意外有效): 向量维度=%d, 向量数=%d", e.Dimensions(), len(vectors))
		return
	}
	t.Logf("预期内错误(API Key无效): %v", err)
}

func TestQwenEmbedder_Dimensions(t *testing.T) {
	vc := viper.New()
	vc.Set("embedding.qwen.mode", "dashscope")
	vc.Set("embedding.qwen.dashscope_api_key", "sk-test")

	e := NewQwenEmbedder(vc, newTestLogger())

	if dim := e.Dimensions(); dim != 1024 {
		t.Errorf("期望维度=1024, 实际=%d", dim)
	}

	if name := e.ModelName(); name != "text-embedding-v4" {
		t.Errorf("期望模型名=text-embedding-v4, 实际=%s", name)
	}
}

func TestQwenEmbedder_Ollama(t *testing.T) {
	vc := viper.New()
	vc.Set("embedding.qwen.mode", "ollama")
	vc.Set("embedding.qwen.ollama_endpoint", "http://localhost:11434")
	vc.Set("embedding.qwen.ollama_model", "qwen3-embedding")

	e := NewQwenEmbedder(vc, newTestLogger())

	if e.ModelName() != "qwen3-embedding" {
		t.Fatalf("期望 model=qwen3-embedding, 实际=%s", e.ModelName())
	}

	_, err := e.Embed(context.Background(), []string{"测试文本"})
	if err != nil {
		t.Logf("Ollama 不可达(预期内): %v", err)
	} else {
		t.Log("Ollama 嵌入成功")
	}
}
