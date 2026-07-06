package agent_test

import (
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/log"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func init() {
	// 初始化全局 logger，避免 LoadAgentConfig 调用 log.GetLogger() 时 panic
	log.SetGlobalLogger(&log.Logger{Logger: zap.NewNop()})
}

// TestLoadAgentConfig_Defaults 验证未设置任何配置时，所有字段应用默认值
func TestLoadAgentConfig_Defaults(t *testing.T) {
	vc := viper.New()
	cfg := agent.LoadAgentConfig(vc)

	if cfg.SessionTTLMinutes != 30 {
		t.Errorf("SessionTTLMinutes 默认值应为 30，实际为 %d", cfg.SessionTTLMinutes)
	}
	if cfg.MaxHistoryTurns != 20 {
		t.Errorf("MaxHistoryTurns 默认值应为 20，实际为 %d", cfg.MaxHistoryTurns)
	}
	if cfg.MaxPhaseTurns != 10 {
		t.Errorf("MaxPhaseTurns 默认值应为 10，实际为 %d", cfg.MaxPhaseTurns)
	}
	if cfg.CheckpointCleanupHours != 1 {
		t.Errorf("CheckpointCleanupHours 默认值应为 1，实际为 %d", cfg.CheckpointCleanupHours)
	}
	if cfg.LLMMaxRetries != 3 {
		t.Errorf("LLMMaxRetries 默认值应为 3，实际为 %d", cfg.LLMMaxRetries)
	}
	if cfg.LLMRetryBackoffSeconds != 2 {
		t.Errorf("LLMRetryBackoffSeconds 默认值应为 2，实际为 %d", cfg.LLMRetryBackoffSeconds)
	}
	if cfg.ToolTimeoutSeconds != 30 {
		t.Errorf("ToolTimeoutSeconds 默认值应为 30，实际为 %d", cfg.ToolTimeoutSeconds)
	}
	if cfg.IntentConfidenceThreshold != 0.7 {
		t.Errorf("IntentConfidenceThreshold 默认值应为 0.7，实际为 %f", cfg.IntentConfidenceThreshold)
	}
}

// TestLoadAgentConfig_CustomValues 验证自定义值覆盖所有默认值
func TestLoadAgentConfig_CustomValues(t *testing.T) {
	vc := viper.New()
	vc.Set("agent.session_ttl_minutes", 60)
	vc.Set("agent.max_history_turns", 50)
	vc.Set("agent.max_phase_turns", 30)
	vc.Set("agent.checkpoint_cleanup_hours", 24)
	vc.Set("agent.llm_max_retries", 5)
	vc.Set("agent.llm_retry_backoff_seconds", 5)
	vc.Set("agent.tool_timeout_seconds", 60)
	vc.Set("agent.intent_confidence_threshold", 0.85)

	cfg := agent.LoadAgentConfig(vc)

	if cfg.SessionTTLMinutes != 60 {
		t.Errorf("SessionTTLMinutes 应为 60，实际为 %d", cfg.SessionTTLMinutes)
	}
	if cfg.MaxHistoryTurns != 50 {
		t.Errorf("MaxHistoryTurns 应为 50，实际为 %d", cfg.MaxHistoryTurns)
	}
	if cfg.MaxPhaseTurns != 30 {
		t.Errorf("MaxPhaseTurns 应为 30，实际为 %d", cfg.MaxPhaseTurns)
	}
	if cfg.CheckpointCleanupHours != 24 {
		t.Errorf("CheckpointCleanupHours 应为 24，实际为 %d", cfg.CheckpointCleanupHours)
	}
	if cfg.LLMMaxRetries != 5 {
		t.Errorf("LLMMaxRetries 应为 5，实际为 %d", cfg.LLMMaxRetries)
	}
	if cfg.LLMRetryBackoffSeconds != 5 {
		t.Errorf("LLMRetryBackoffSeconds 应为 5，实际为 %d", cfg.LLMRetryBackoffSeconds)
	}
	if cfg.ToolTimeoutSeconds != 60 {
		t.Errorf("ToolTimeoutSeconds 应为 60，实际为 %d", cfg.ToolTimeoutSeconds)
	}
	if cfg.IntentConfidenceThreshold != 0.85 {
		t.Errorf("IntentConfidenceThreshold 应为 0.85，实际为 %f", cfg.IntentConfidenceThreshold)
	}
}

// TestLoadAgentConfig_PartialOverride 验证部分字段覆盖时，未设置字段使用默认值
func TestLoadAgentConfig_PartialOverride(t *testing.T) {
	vc := viper.New()
	// 只设置 3 个字段
	vc.Set("agent.session_ttl_minutes", 45)
	vc.Set("agent.llm_max_retries", 8)
	vc.Set("agent.tool_timeout_seconds", 45)

	cfg := agent.LoadAgentConfig(vc)

	// 自定义值验证
	if cfg.SessionTTLMinutes != 45 {
		t.Errorf("SessionTTLMinutes 应为 45，实际为 %d", cfg.SessionTTLMinutes)
	}
	if cfg.LLMMaxRetries != 8 {
		t.Errorf("LLMMaxRetries 应为 8，实际为 %d", cfg.LLMMaxRetries)
	}
	if cfg.ToolTimeoutSeconds != 45 {
		t.Errorf("ToolTimeoutSeconds 应为 45，实际为 %d", cfg.ToolTimeoutSeconds)
	}

	// 未设置字段应为默认值
	if cfg.MaxHistoryTurns != 20 {
		t.Errorf("MaxHistoryTurns 默认值应为 20，实际为 %d", cfg.MaxHistoryTurns)
	}
	if cfg.MaxPhaseTurns != 10 {
		t.Errorf("MaxPhaseTurns 默认值应为 10，实际为 %d", cfg.MaxPhaseTurns)
	}
	if cfg.CheckpointCleanupHours != 1 {
		t.Errorf("CheckpointCleanupHours 默认值应为 1，实际为 %d", cfg.CheckpointCleanupHours)
	}
	if cfg.LLMRetryBackoffSeconds != 2 {
		t.Errorf("LLMRetryBackoffSeconds 默认值应为 2，实际为 %d", cfg.LLMRetryBackoffSeconds)
	}
	if cfg.IntentConfidenceThreshold != 0.7 {
		t.Errorf("IntentConfidenceThreshold 默认值应为 0.7，实际为 %f", cfg.IntentConfidenceThreshold)
	}
}

// TestLoadAgentConfig_ZeroValues 验证显式设置为 0 时，仍应用默认值（防止误配）
func TestLoadAgentConfig_ZeroValues(t *testing.T) {
	vc := viper.New()
	vc.Set("agent.session_ttl_minutes", 0)
	vc.Set("agent.max_phase_turns", 0)
	vc.Set("agent.intent_confidence_threshold", 0.0)

	cfg := agent.LoadAgentConfig(vc)

	// 显式设为 0 的字段应回退到默认值（不是 0）
	if cfg.SessionTTLMinutes != 30 {
		t.Errorf("SessionTTLMinutes 设为 0 时应回退到默认值 30，实际为 %d", cfg.SessionTTLMinutes)
	}
	if cfg.MaxPhaseTurns != 10 {
		t.Errorf("MaxPhaseTurns 设为 0 时应回退到默认值 10，实际为 %d", cfg.MaxPhaseTurns)
	}
	if cfg.IntentConfidenceThreshold != 0.7 {
		t.Errorf("IntentConfidenceThreshold 设为 0 时应回退到默认值 0.7，实际为 %f", cfg.IntentConfidenceThreshold)
	}
}
