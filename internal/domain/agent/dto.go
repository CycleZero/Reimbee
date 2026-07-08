// Package agent 数据传输对象
package agent

import "time"

// ListSessionsResponse 会话列表游标分页响应
type ListSessionsResponse struct {
	Sessions   []SessionItem `json:"sessions"`
	NextCursor string        `json:"next_cursor"` // 下一页游标，空表示无更多
	HasMore    bool          `json:"has_more"`
}

// SessionItem 会话列表项
type SessionItem struct {
	SessionID    string    `json:"session_id"`
	Status       string    `json:"status"`
	Summary      string    `json:"summary"`
	MessageCount int       `json:"message_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// GetMessagesResponse 会话消息响应（全量）
type GetMessagesResponse struct {
	Messages []MessageItem `json:"messages"`
}

// MessageItem 消息项（仅返回展示字段，不含完整 Parts）
type MessageItem struct {
	Seq       uint   `json:"seq"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	ToolName  string `json:"tool_name,omitempty"`
	CreatedAt string `json:"created_at"`
}
