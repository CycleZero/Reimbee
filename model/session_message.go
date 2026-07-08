// Package model 会话消息持久化模型
// v5: MessageMeta → BladesJSON，新增 ToolCallID，兼容 blades.Message JSON 往返
package model

import "time"

// SessionMessage 会话消息明细持久化记录
//
// v5 变更：
//   - MessageMeta → BladesJSON: 语义从"Eino元数据"切换为"blades.Message完整JSON"
//   - 新增 ToolCallID: 关联 tool call 与 tool result（ToolPart.ID）
type SessionMessage struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID string `gorm:"type:varchar(36);index:idx_session_seq,priority:1;not null;comment:会话ID"`
	Seq       uint   `gorm:"index:idx_session_seq,priority:2;not null;comment:消息序号(会话内递增)"`
	Role      string `gorm:"type:varchar(20);not null;comment:角色(user/assistant/tool/system)"`

	// 结构化摘要（查询/展示用，LLM不走此列）
	Content     *string `gorm:"type:text;comment:TextParts拼接摘要"`
	ToolName    string  `gorm:"type:varchar(64);not null;default:'';comment:工具名称"`
	ToolCallID  string  `gorm:"type:varchar(64);not null;default:'';comment:ToolPart.ID(关联call与result)"`
	ToolRequest *string `gorm:"type:text;comment:ToolPart.Request(LLM调用参数)"`
	ToolOutput  *string `gorm:"type:text;comment:ToolPart.Response(工具返回结果)"`

	// blades.Message 完整JSON — LLM 上下文唯一来源
	BladesJSON *string `gorm:"type:json;comment:blades.Message完整JSON(含Parts/Actions/Metadata/TokenUsage)"`

	CreatedAt time.Time `gorm:"autoCreateTime;comment:创建时间"`
}

// TableName 指定 GORM 表名
func (SessionMessage) TableName() string {
	return "session_messages"
}
