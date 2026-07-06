package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ============================================
// CheckpointStore 接口（Eino compose.CheckpointStore 兼容）
// ============================================

// CheckpointStore Eino Graph Checkpoint 持久化接口
// 实现此接口以支持 Graph 的暂停/恢复和中断/继续功能
type CheckpointStore interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
}

// ============================================
// MySQL 实现
// ============================================

// CheckpointRecord Checkpoint 持久化 GORM 模型
type CheckpointRecord struct {
	ID        string    `gorm:"primaryKey;type:varchar(128);comment:CheckpointID（格式: GraphName:SessionID）"`
	Data      string    `gorm:"type:mediumtext;not null;comment:Checkpoint 序列化 JSON"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定 GORM 表名
func (CheckpointRecord) TableName() string {
	return "checkpoint_records"
}

// MySQLCheckpointStore 基于 MySQL 的 Checkpoint 持久化实现
// 每个 Checkpoint 以 JSON 序列化后存入 MySQL，key 格式为 GraphName:SessionID
type MySQLCheckpointStore struct {
	db     *gorm.DB
	logger *log.Logger
}

// NewMySQLCheckpointStore 创建 MySQL Checkpoint 存储实例，自动迁移表结构
func NewMySQLCheckpointStore(db *gorm.DB, logger *log.Logger) *MySQLCheckpointStore {
	if err := db.AutoMigrate(&CheckpointRecord{}); err != nil {
		logger.Error("迁移 Checkpoint 表失败", zap.Error(err))
		panic("迁移 Checkpoint 表失败: " + err.Error())
	}
	logger.Debug("Checkpoint 存储初始化完成（MySQL）")
	return &MySQLCheckpointStore{db: db, logger: logger}
}

// Get 从 MySQL 读取 Checkpoint 数据
// 返回 (nil, false, nil) 表示 Checkpoint 不存在
func (s *MySQLCheckpointStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	var record CheckpointRecord
	err := s.db.WithContext(ctx).First(&record, "id = ?", key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logger.Debug("Checkpoint不存在", zap.String("key", key))
			return nil, false, nil
		}
		s.logger.Error("查询Checkpoint失败", zap.String("key", key), zap.Error(err))
		return nil, false, fmt.Errorf("查询Checkpoint失败: %w", err)
	}

	s.logger.Debug("Checkpoint读取成功", zap.String("key", key), zap.Int("数据大小(bytes)", len(record.Data)))
	return []byte(record.Data), true, nil
}

// Set 将 Checkpoint 数据存入 MySQL（UPSERT 语义）
func (s *MySQLCheckpointStore) Set(ctx context.Context, key string, value []byte) error {
	record := CheckpointRecord{
		ID:        key,
		Data:      string(value),
		UpdatedAt: time.Now(),
	}

	// 使用 Save 实现 INSERT OR UPDATE
	if err := s.db.WithContext(ctx).Save(&record).Error; err != nil {
		s.logger.Error("保存Checkpoint失败", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("保存Checkpoint失败: %w", err)
	}

	s.logger.Debug("Checkpoint保存成功", zap.String("key", key), zap.Int("数据大小(bytes)", len(value)))
	return nil
}

// Delete 从 MySQL 删除 Checkpoint
func (s *MySQLCheckpointStore) Delete(ctx context.Context, key string) error {
	result := s.db.WithContext(ctx).Delete(&CheckpointRecord{}, "id = ?", key)
	if result.Error != nil {
		s.logger.Error("删除Checkpoint失败", zap.String("key", key), zap.Error(result.Error))
		return fmt.Errorf("删除Checkpoint失败: %w", result.Error)
	}

	s.logger.Debug("Checkpoint删除成功", zap.String("key", key), zap.Int64("影响行数", result.RowsAffected))
	return nil
}

// ============================================
// Checkpoint 清理
// ============================================

// CleanOrphanCheckpoints 清理超过指定时长的孤儿 Checkpoint
// maxAge: 最大保留时长（小时），超过此时间的 Checkpoint 视为孤儿
func (s *MySQLCheckpointStore) CleanOrphanCheckpoints(ctx context.Context, maxAgeHours int) error {
	cutoff := time.Now().Add(-time.Duration(maxAgeHours) * time.Hour)

	result := s.db.WithContext(ctx).
		Where("updated_at < ?", cutoff).
		Delete(&CheckpointRecord{})

	if result.Error != nil {
		s.logger.Error("清理孤儿Checkpoint失败", zap.Error(result.Error))
		return fmt.Errorf("清理孤儿Checkpoint失败: %w", result.Error)
	}

	s.logger.Info("孤儿Checkpoint清理完成", zap.Int64("清理数量", result.RowsAffected))
	return nil
}

// ============================================
// 序列化辅助
// ============================================

// MarshalCheckpoint 将 Checkpoint 数据序列化为 JSON 字节
func MarshalCheckpoint(data any) ([]byte, error) {
	return json.Marshal(data)
}

// UnmarshalCheckpoint 将 JSON 字节反序列化为目标对象
func UnmarshalCheckpoint(data []byte, target any) error {
	return json.Unmarshal(data, target)
}
