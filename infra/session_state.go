package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// SessionState GORM 模型，持久化会话级业务状态（如 ReimbursementState）
// 每个 session 按 key 隔离不同业务域的状态，id = sessionID:key 确保唯一
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

// SaveState 将业务状态 JSON 序列化后持久化到 MySQL
func (s *MySQLSessionStore) SaveState(ctx context.Context, sessionID string, key string, state any) error {
	if state == nil {
		return fmt.Errorf("状态对象不能为 nil")
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("序列化状态[%s]失败: %w", key, err)
	}

	record := SessionState{
		ID:        sessionID + ":" + key,
		SessionID: sessionID,
		StateKey:  key,
		Data:      string(data),
		UpdatedAt: time.Now(),
	}

	if err := s.db.WithContext(ctx).Save(&record).Error; err != nil {
		return fmt.Errorf("保存状态[%s]到MySQL失败: %w", key, err)
	}
	return nil
}

// GetState 从 MySQL 读取并反序列化状态数据
func (s *MySQLSessionStore) GetState(ctx context.Context, sessionID string, key string, target any) (bool, error) {
	var record SessionState
	err := s.db.WithContext(ctx).
		Where("id = ?", sessionID+":"+key).
		First(&record).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, fmt.Errorf("查询状态[%s]失败: %w", key, err)
	}

	if err := json.Unmarshal([]byte(record.Data), target); err != nil {
		return false, fmt.Errorf("反序列化状态[%s]失败: %w", key, err)
	}
	return true, nil
}

// DeleteState 从 MySQL 删除指定状态
func (s *MySQLSessionStore) DeleteState(ctx context.Context, sessionID string, key string) error {
	return s.db.WithContext(ctx).
		Where("id = ?", sessionID+":"+key).
		Delete(&SessionState{}).Error
}
