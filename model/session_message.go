package model

import "time"

// SessionMessage 会话消息持久化记录
type SessionMessage struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID string    `gorm:"type:varchar(36);index:idx_session_time,priority:1;not null;comment:会话ID(UUID v7)" json:"session_id"`
	Role      string    `gorm:"type:varchar(20);not null;comment:user/assistant/tool" json:"role"`
	Content   string    `gorm:"type:text;comment:消息文本内容" json:"content"`
	RawJSON   string    `gorm:"type:mediumtext;not null;comment:完整Message JSON(含ToolCalls等)" json:"raw_json"`
	CreatedAt time.Time `gorm:"autoCreateTime;index:idx_session_time,priority:2;comment:创建时间" json:"created_at"`
}
