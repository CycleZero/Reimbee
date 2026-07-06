package infra

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// SessionStore 会话持久化接口
// 底层存储 Eino 原生的 []*schema.Message 切片（JSON 序列化），
// 包含完整的 tool_calls / tool_call_id 等元数据
type SessionStore interface {
	// SaveMessages 保存一轮完整交互的消息（user + assistant + tool 消息）
	SaveMessages(ctx context.Context, sessionID string, msgs []*schema.Message) error

	// GetHistory 获取会话最近的消息（按时间正序）
	GetHistory(ctx context.Context, sessionID string, limit int) ([]*schema.Message, error)

	// Clear 清除会话所有消息
	Clear(ctx context.Context, sessionID string) error
}
