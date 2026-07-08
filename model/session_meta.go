// Package model 会话元数据持久化模型
// v5: 补充 Summary 字段
package model

import "time"

// SessionMeta 会话元数据持久化记录
type SessionMeta struct {
	ID           uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID    string     `gorm:"type:varchar(36);uniqueIndex;not null;comment:会话UUID"`
	UserID       uint       `gorm:"not null;index;comment:用户ID"`
	EmployeeID   string     `gorm:"type:varchar(32);not null;default:'';comment:员工工号"`
	EmployeeName string     `gorm:"type:varchar(64);not null;default:'';comment:员工姓名"`
	Role         string     `gorm:"type:varchar(20);not null;default:'employee';comment:角色"`
	Status       string     `gorm:"type:varchar(20);not null;default:'active';comment:会话状态"`
	Summary      string     `gorm:"type:varchar(512);not null;default:'';comment:会话摘要"`
	MessageCount uint       `gorm:"not null;default:0;comment:消息总数(冗余计数器)"`
	CreatedAt    time.Time  `gorm:"autoCreateTime;comment:创建时间"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime;comment:最后活跃时间"`
	ExpiresAt    *time.Time `gorm:"index;comment:过期时间"`
}

func (SessionMeta) TableName() string {
	return "session_meta"
}

const (
	SessionStatusActive    = "active"
	SessionStatusCompleted = "completed"
	SessionStatusExpired   = "expired"
)
