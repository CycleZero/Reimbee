// Package model Session 会话消息持久化模型
//
// v4 重构：从单一 RawJSON 字段拆分为结构化字段，
// 支持按字段查询（content、tool_name、tool_output），
// 保留 MessageMeta 存储完整 Eino Message 元数据（ToolCalls 等，仅框架消费）。
package model

import "time"

// SessionMessage 会话消息明细持久化记录
//
// v3 到 v4 变更：
//   - RawJSON → MessageMeta: 改名为"元数据"，语义更精准
//   - 新增 Seq: 会话内单调递增序号，保证消息顺序（替代依赖 created_at 排序）
//   - 新增结构化字段: ToolName / ToolInput / ToolOutput（仅 tool 角色填充）
//   - Content 改为 *string: 允许 NULL，区分空消息和未设置
type SessionMessage struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID string    `gorm:"type:varchar(36);index:idx_session_seq,priority:1;not null;comment:会话ID(UUID v7)" json:"session_id"`
	Seq       uint      `gorm:"index:idx_session_seq,priority:2;not null;comment:消息序号(会话内递增，保证顺序)" json:"seq"`
	Role      string    `gorm:"type:varchar(20);index:idx_session_role,priority:2;not null;comment:角色(user/assistant/tool)" json:"role"`

	// ── 结构化内容（v4 新增，替代旧 RawJSON）──

	// Content 消息文本内容
	//   user: 用户输入原文
	//   assistant: LLM 回复原文（可能为空，如仅工具调用）
	//   tool: 工具返回摘要（截断前 500 字符，全量存 ToolOutput）
	Content *string `gorm:"type:text;comment:消息文本(user=用户输入/assistant=LLM回复/tool=工具返回摘要)" json:"content,omitempty"`

	// ToolName 工具名称（仅 tool 角色填充）
	ToolName string `gorm:"type:varchar(64);not null;default:'';comment:工具名称(仅tool角色)" json:"tool_name,omitempty"`

	// ToolInput 工具调用输入参数 JSON（仅 tool 角色填充）
	// 存储 LLM 调用工具时传入的完整 JSON 参数
	ToolInput *string `gorm:"type:text;comment:工具输入参数JSON(仅tool角色)" json:"tool_input,omitempty"`

	// ToolOutput 工具调用输出结果 JSON（仅 tool 角色填充）
	// 存储工具返回的完整结果
	ToolOutput *string `gorm:"type:text;comment:工具输出结果JSON(仅tool角色)" json:"tool_output,omitempty"`

	// ── Eino 框架元数据（仅框架消费，业务不直接查询）──

	// MessageMeta Eino Message 完整元数据 JSON
	// 包含 ToolCalls、ResponseMeta、ReasoningContent 等框架专用字段
	// 用于还原为 *schema.Message 时保留所有信息
	MessageMeta *string `gorm:"type:json;comment:Eino Message元数据(ToolCalls/ResponseMeta等，仅框架消费)" json:"message_meta,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime;comment:创建时间" json:"created_at"`
}

// TableName 指定 GORM 表名
func (SessionMessage) TableName() string {
	return "session_messages"
}
