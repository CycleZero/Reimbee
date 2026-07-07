// Package model Session 会话元数据持久化模型
//
// v4 重构：将会话级别信息从 session_messages 中分离，
// 形成 session_meta（元数据）+ session_messages（消息明细）双表结构。
// 元数据表存储用户身份、会话状态、CheckpointID 等会话级信息。
package model

import "time"

// SessionMeta 会话元数据持久化记录
//
// 与 session_messages 一对多关系，一条 meta 对应多条 message。
// 元数据在会话创建时写入，后续每次 Push 消息时更新计数器。
//
// 表名: session_meta
type SessionMeta struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID    string     `gorm:"type:varchar(36);uniqueIndex;not null;comment:会话UUID v7" json:"session_id"`
	UserID       uint       `gorm:"not null;index;comment:用户ID（关联auth.users）" json:"user_id"`
	EmployeeID   string     `gorm:"type:varchar(32);not null;default:'';comment:员工工号" json:"employee_id"`
	Role         string     `gorm:"type:varchar(20);not null;default:'employee';comment:角色（employee/approver/admin）" json:"role"`
	Status       string     `gorm:"type:varchar(20);not null;default:'active';comment:会话状态（active/completed/expired）" json:"status"`
	Summary      string     `gorm:"type:varchar(512);not null;default:'';comment:会话摘要（截取最后一条用户消息，用于列表展示）" json:"summary"`
	CheckpointID string     `gorm:"type:varchar(128);not null;default:'';comment:Eino CheckpointID（v4 Interrupt 恢复使用）" json:"checkpoint_id"`
	MessageCount uint       `gorm:"not null;default:0;comment:消息总数（冗余计数器，避免 COUNT 查询）" json:"message_count"`
	CreatedAt    time.Time  `gorm:"autoCreateTime;comment:会话创建时间" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime;comment:最后活跃时间" json:"updated_at"`
	ExpiresAt    *time.Time `gorm:"index;comment:会话过期时间（NULL=永不过期，超出后 cleanupLoop 清理）" json:"expires_at,omitempty"`
}

// TableName 指定 GORM 表名
func (SessionMeta) TableName() string {
	return "session_meta"
}

// 会话状态常量
const (
	SessionStatusActive    = "active"    // 活跃中
	SessionStatusCompleted = "completed" // 已完成
	SessionStatusExpired   = "expired"   // 已过期
)
