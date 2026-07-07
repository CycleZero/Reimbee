package infra

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/CycleZero/Reimbee/model"
)

// SessionMetaRepo 会话元数据仓储
//
// 薄封装层，直接操作 GORM，提供 session_meta 表的 CRUD。
// 不实现业务逻辑，仅封装数据库访问。
type SessionMetaRepo struct {
	db *gorm.DB
}

func NewSessionMetaRepo(db *gorm.DB) *SessionMetaRepo {
	return &SessionMetaRepo{db: db}
}

// AutoMigrate 确保 session_meta 表存在
func (r *SessionMetaRepo) AutoMigrate() error {
	return r.db.AutoMigrate(&model.SessionMeta{})
}

// Create 创建会话元数据记录
func (r *SessionMetaRepo) Create(ctx context.Context, meta *model.SessionMeta) error {
	return r.db.WithContext(ctx).Create(meta).Error
}

// GetBySessionID 按 sessionID 查询元数据
func (r *SessionMetaRepo) GetBySessionID(ctx context.Context, sessionID string) (*model.SessionMeta, error) {
	var meta model.SessionMeta
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&meta).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("查询会话元数据失败: %w", err)
	}
	return &meta, nil
}

// Update 更新会话元数据指定字段
func (r *SessionMetaRepo) Update(ctx context.Context, sessionID string, updates map[string]any) error {
	return r.db.WithContext(ctx).
		Model(&model.SessionMeta{}).
		Where("session_id = ?", sessionID).
		Updates(updates).Error
}

// Delete 删除会话元数据
func (r *SessionMetaRepo) Delete(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&model.SessionMeta{}).Error
}

// List 查询用户会话列表（按更新时间倒序，最多 50 条）
func (r *SessionMetaRepo) List(ctx context.Context, userID uint, status string) ([]*model.SessionMeta, error) {
	var metas []*model.SessionMeta
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Order("updated_at DESC").Limit(50).Find(&metas).Error; err != nil {
		return nil, fmt.Errorf("查询会话列表失败: %w", err)
	}
	return metas, nil
}

// IncrementMessageCount 原子递增消息计数
func (r *SessionMetaRepo) IncrementMessageCount(ctx context.Context, sessionID string, delta uint) error {
	return r.db.WithContext(ctx).
		Model(&model.SessionMeta{}).
		Where("session_id = ?", sessionID).
		UpdateColumn("message_count", gorm.Expr("message_count + ?", delta)).Error
}

// UpdateSummary 更新会话摘要
func (r *SessionMetaRepo) UpdateSummary(ctx context.Context, sessionID string, summary string) error {
	return r.db.WithContext(ctx).
		Model(&model.SessionMeta{}).
		Where("session_id = ?", sessionID).
		Update("summary", summary).Error
}

// GetDB 获取底层 *gorm.DB（供 Wire 注入复用）
func (r *SessionMetaRepo) GetDB() *gorm.DB {
	return r.db
}
