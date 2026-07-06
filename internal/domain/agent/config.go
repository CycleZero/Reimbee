package agent

import (
	"github.com/CycleZero/Reimbee/log"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// AgentConfig 智能体运行时配置，从 config.yaml 的 agent: 段加载
// 所有配置项均有合理默认值，配置文件缺失时不影响启动
type AgentConfig struct {
	SessionTTLMinutes         int     // 会话超时时间（分钟），超过后会话标记为过期
	MaxHistoryTurns           int     // 每次 Graph 执行时注入的最大历史对话轮数
	MaxPhaseTurns             int     // 每个 Phase 内 Agent 的最大交互轮次（防死循环）
	CheckpointCleanupHours    int     // 孤儿 Checkpoint 清理周期（小时），超时未更新的 Checkpoint 将被清除
	LLMMaxRetries             int     // LLM API 调用最大重试次数
	LLMRetryBackoffSeconds    int     // 重试退避初始间隔（秒），指数退避：1s → 2s → 4s
	ToolTimeoutSeconds        int     // 工具调用超时时间（秒），超时后返回错误不阻塞流程
	IntentConfidenceThreshold float64 // 意图分类置信度阈值，低于此值时询问用户确认意图
}

// LoadAgentConfig 从 Viper 加载 Agent 配置，所有配置项带默认值
// 配置文件缺失或字段未设置时使用安全默认值，不 panic
func LoadAgentConfig(vc *viper.Viper) *AgentConfig {
	logger := log.GetLogger()

	cfg := &AgentConfig{
		SessionTTLMinutes:         vc.GetInt("agent.session_ttl_minutes"),
		MaxHistoryTurns:           vc.GetInt("agent.max_history_turns"),
		MaxPhaseTurns:             vc.GetInt("agent.max_phase_turns"),
		CheckpointCleanupHours:    vc.GetInt("agent.checkpoint_cleanup_hours"),
		LLMMaxRetries:             vc.GetInt("agent.llm_max_retries"),
		LLMRetryBackoffSeconds:    vc.GetInt("agent.llm_retry_backoff_seconds"),
		ToolTimeoutSeconds:        vc.GetInt("agent.tool_timeout_seconds"),
		IntentConfidenceThreshold: vc.GetFloat64("agent.intent_confidence_threshold"),
	}

	// 为未配置的项设置生产级默认值
	applyDefaults(cfg)

	logger.Debug("智能体配置加载完成",
		zap.Int("会话超时(分)", cfg.SessionTTLMinutes),
		zap.Int("最大历史轮数", cfg.MaxHistoryTurns),
		zap.Int("最大Phase轮次", cfg.MaxPhaseTurns),
		zap.Int("Checkpoint清理(时)", cfg.CheckpointCleanupHours),
		zap.Int("LLM最大重试", cfg.LLMMaxRetries),
		zap.Float64("意图置信度阈值", cfg.IntentConfidenceThreshold),
	)

	return cfg
}

// applyDefaults 为值为零的配置项设置生产级安全默认值
func applyDefaults(cfg *AgentConfig) {
	if cfg.SessionTTLMinutes <= 0 {
		cfg.SessionTTLMinutes = 30
	}
	if cfg.MaxHistoryTurns <= 0 {
		cfg.MaxHistoryTurns = 20
	}
	if cfg.MaxPhaseTurns <= 0 {
		cfg.MaxPhaseTurns = 10
	}
	if cfg.CheckpointCleanupHours <= 0 {
		cfg.CheckpointCleanupHours = 1
	}
	if cfg.LLMMaxRetries <= 0 {
		cfg.LLMMaxRetries = 3
	}
	if cfg.LLMRetryBackoffSeconds <= 0 {
		cfg.LLMRetryBackoffSeconds = 2
	}
	if cfg.ToolTimeoutSeconds <= 0 {
		cfg.ToolTimeoutSeconds = 30
	}
	if cfg.IntentConfidenceThreshold <= 0 {
		cfg.IntentConfidenceThreshold = 0.7
	}
}
