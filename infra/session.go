package infra

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// SessionStore 会话持久化接口
// 管理对话消息历史和业务状态（如 ReimbursementState）的持久化，
// 支持 MySQL 主存储 + Redis 缓存加速

// StateKey 定义 SessionStore 中持久化状态的键名
const (
	StateKeyReimbursement = "reimbursement" // 报销流程状态 agent.ReimbursementState
	StateKeyUserIdentity  = "user_identity" // 用户身份信息 (user_id, employee_id, role)
)

type SessionStore interface {
	// SaveMessages 保存一轮完整交互的消息（user + assistant + tool 消息）
	SaveMessages(ctx context.Context, sessionID string, msgs []*schema.Message) error

	// GetHistory 获取会话最近的消息（按时间正序）
	GetHistory(ctx context.Context, sessionID string, limit int) ([]*schema.Message, error)

	// Clear 清除会话所有消息
	Clear(ctx context.Context, sessionID string) error

	// SaveState 持久化会话的业务状态，按 sessionID + key 唯一标识
	// key: 状态标识（如 "reimbursement"）
	// state: 待序列化的状态对象（需支持 json.Marshal）
	SaveState(ctx context.Context, sessionID string, key string, state any) error

	// GetState 获取已持久化的业务状态，反序列化到 target 指针
	// 返回 (true, nil) 表示状态存在且已反序列化成功
	// 返回 (false, nil) 表示状态不存在（首次请求）
	GetState(ctx context.Context, sessionID string, key string, target any) (bool, error)

	// DeleteState 删除指定 session 的指定业务状态
	// 通常在报销流程完成或会话清理时调用
	DeleteState(ctx context.Context, sessionID string, key string) error
}
