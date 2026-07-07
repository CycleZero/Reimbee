// Package infra 提供基础设施层组件：数据库、Redis、OCR、文件存储、会话持久化。
//
// SessionStore 接口管理对话消息和会话元数据的持久化，支持 MySQL 主存储 + Redis 缓存加速。
// v4 新增 session_meta 表分离会话元数据，并支持 Checkpoint 快照用于 Interrupt 恢复。
package infra

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// SessionStore 会话持久化接口（v4 重构）
//
// 实现: MySQLSessionStore（MySQL 主存储 + Redis 缓存层）
type SessionStore interface {
	// ── 消息持久化 ──
	// 写入时自动分配 seq（会话内递增），更新 meta.message_count

	// SaveMessages 保存一批消息到指定会话
	// 内部自动分配 seq 序号，更新 session_meta.message_count
	SaveMessages(ctx context.Context, sessionID string, msgs []*schema.Message) error

	// GetHistory 获取会话历史消息（按 seq 正序）
	// limit: 返回最近 N 条（0 表示全部）
	GetHistory(ctx context.Context, sessionID string, limit int) ([]*schema.Message, error)

	// Clear 清除指定会话的全部消息（同时清理缓存）
	Clear(ctx context.Context, sessionID string) error

	// ── v4 新增：会话元数据操作 ──

	// CreateSession 创建新会话，写入 session_meta 表
	// 首次对话时由 Handler 调用，后续 Push 时仅更新 message_count
	CreateSession(ctx context.Context, meta *SessionMeta) error

	// GetSession 获取会话元数据
	// 返回 (nil, nil) 表示会话不存在
	GetSession(ctx context.Context, sessionID string) (*SessionMeta, error)

	// UpdateSession 更新会话元数据字段
	// updates: 只更新提供的字段（map[string]any），不覆盖未提供的字段
	UpdateSession(ctx context.Context, sessionID string, updates map[string]any) error

	// DeleteSession 删除会话（meta + messages 联删或标记过期）
	DeleteSession(ctx context.Context, sessionID string) error

	// ListSessions 查询用户会话列表
	// status: 空字符串表示查询全部状态
	ListSessions(ctx context.Context, userID uint, status string) ([]*SessionMeta, error)

	// ── v4 保留：Checkpoint 状态快照（仅 Interrupt 恢复时使用）──

	// SaveCheckpointState 保存 Checkpoint 快照
	// key: 状态标识（如 "reimbursement"），用于 Interrupt 时暂存业务状态
	SaveCheckpointState(ctx context.Context, sessionID string, key string, state any) error

	// GetCheckpointState 获取 Checkpoint 快照
	// 返回 (true, nil) 表示状态存在且反序列化成功
	GetCheckpointState(ctx context.Context, sessionID string, key string, target any) (bool, error)

	// DeleteCheckpointState 删除 Checkpoint 快照
	DeleteCheckpointState(ctx context.Context, sessionID string, key string) error

	// ── 兼容别名（v3→v4 过渡期使用，Phase 5 清理时移除）──
	// SaveState → SaveCheckpointState
	// GetState  → GetCheckpointState
	// DeleteState → DeleteCheckpointState
	SaveState(ctx context.Context, sessionID string, key string, state any) error
	GetState(ctx context.Context, sessionID string, key string, target any) (bool, error)
	DeleteState(ctx context.Context, sessionID string, key string) error
}

// StateKey 状态标识常量（v3 兼容，Phase 5 清理时移除）
const (
	StateKeyReimbursement = "reimbursement"
	StateKeyUserIdentity  = "user_identity"
)

// ── 会话元数据传输对象 ──

// SessionMeta 会话元数据（接口层 DTO，与 model.SessionMeta 对应）
type SessionMeta struct {
	SessionID    string `json:"session_id"`
	UserID       uint   `json:"user_id"`
	EmployeeID   string `json:"employee_id"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	Summary      string `json:"summary"`
	CheckpointID string `json:"checkpoint_id"`
	MessageCount uint   `json:"message_count"`
}

// 会话状态常量（与 model 包同步）
const (
	SessionStatusActive    = "active"
	SessionStatusCompleted = "completed"
	SessionStatusExpired   = "expired"
)
