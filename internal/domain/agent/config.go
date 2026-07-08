// Package agent 配置
package agent

import "time"

// Config ReimburseAgent 运行时配置
type Config struct {
	MaxHistoryTurns int           // History 最大加载轮数
	SessionTTL      time.Duration // 会话超时时间
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxHistoryTurns: 20,
		SessionTTL:      30 * time.Minute,
	}
}
