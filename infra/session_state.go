package infra

import "time"

// SessionState GORM 模型，持久化 Checkpoint 快照
// v3→v4: SaveState/GetState/DeleteState 方法移至 session_mysql.go 作为兼容别名
type SessionState struct {
	ID        string    `gorm:"primaryKey;type:varchar(256);comment:唯一标识(sessionID:key)"`
	SessionID string    `gorm:"type:varchar(128);index;not null;comment:会话ID"`
	StateKey  string    `gorm:"type:varchar(64);not null;comment:状态键(reimbursement等)"`
	Data      string    `gorm:"type:mediumtext;not null;comment:JSON序列化的状态数据"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (SessionState) TableName() string {
	return "session_states"
}
