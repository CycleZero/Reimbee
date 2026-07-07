// Package infra 会话消息 MySQL 持久化实现（v4 重构）
//
// 从单表模式重构为 meta + messages 双表：
//   - session_meta: 会话元数据（用户身份、状态、CheckpointID、消息计数）
//   - session_messages: 消息明细（结构化字段，支持按 role/tool 查询）
//
// 读写路径：优先 Redis 缓存 → 未命中回源 MySQL → 异步预热缓存
package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/CycleZero/Reimbee/model"
	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MySQLSessionStore MySQL 会话持久化实现
// v4 结构：持有 metaRepo（元数据操作）+ db（消息操作）+ cache（Redis 缓存层）
type MySQLSessionStore struct {
	db       *gorm.DB
	metaRepo *SessionMetaRepo
	cache    *RedisSessionCache
	logger   *log.Logger
}

// NewMySQLSessionStore 创建 MySQL 会话存储实例，自动迁移表结构
func NewMySQLSessionStore(data *Data, cache *RedisSessionCache, metaRepo *SessionMetaRepo, logger *log.Logger) *MySQLSessionStore {
	store := &MySQLSessionStore{
		db:       data.DB,
		metaRepo: metaRepo,
		cache:    cache,
		logger:   logger,
	}

	if err := store.db.AutoMigrate(&model.SessionMessage{}); err != nil {
		panic(fmt.Errorf("会话消息表自动迁移失败: %w", err))
	}
	if err := metaRepo.AutoMigrate(); err != nil {
		panic(fmt.Errorf("会话元数据表自动迁移失败: %w", err))
	}
	// 保留旧 session_states 表用于 Checkpoint 快照（不再使用 AutoMigrate 因为接口已变）
	store.db.AutoMigrate(&SessionState{})

	logger.Info("会话存储初始化完成（v4: meta + messages 双表）")
	return store
}

// ============================================
// 消息持久化
// ============================================

// SaveMessages 将 Eino Message 拆分为结构化字段批量写入
func (s *MySQLSessionStore) SaveMessages(ctx context.Context, sessionID string, msgs []*schema.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	var maxSeq uint
	s.db.WithContext(ctx).Model(&model.SessionMessage{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq)

	records := make([]*model.SessionMessage, 0, len(msgs))
	for i, msg := range msgs {
		rec := &model.SessionMessage{
			SessionID: sessionID,
			Seq:       maxSeq + uint(i) + 1,
			Role:      string(msg.Role),
		}

		switch msg.Role {
		case schema.User, schema.Assistant:
			content := msg.Content
			rec.Content = &content
		case schema.Tool:
			rec.ToolName = msg.ToolName
			if msg.Content != "" {
				summary := truncateContent(msg.Content, 500)
				rec.Content = &summary
				output := msg.Content
				rec.ToolOutput = &output
			}
		}

		metaBytes, err := json.Marshal(msg)
		if err != nil {
			s.logger.Warn("序列化消息元数据失败", zap.Error(err))
		} else {
			metaStr := string(metaBytes)
			rec.MessageMeta = &metaStr
		}

		records = append(records, rec)
	}

	if err := s.db.WithContext(ctx).Create(records).Error; err != nil {
		return fmt.Errorf("保存消息失败: %w", err)
	}

	s.metaRepo.IncrementMessageCount(ctx, sessionID, uint(len(msgs)))

	if s.cache != nil {
		go s.cache.Del(context.Background(), sessionID)
	}

	return nil
}

// GetHistory 从结构化字段还原 Eino Message
func (s *MySQLSessionStore) GetHistory(ctx context.Context, sessionID string, limit int) ([]*schema.Message, error) {
	if s.cache != nil {
		if msgs, ok := s.cache.Get(ctx, sessionID); ok {
			if limit > 0 && len(msgs) > limit {
				return msgs[len(msgs)-limit:], nil
			}
			return msgs, nil
		}
	}

	var records []model.SessionMessage

	if limit > 0 {
		if err := s.db.WithContext(ctx).Raw(
			"SELECT * FROM (SELECT * FROM session_messages WHERE session_id = ? ORDER BY seq DESC LIMIT ?) AS recent ORDER BY seq ASC",
			sessionID, limit,
		).Scan(&records).Error; err != nil {
			return nil, fmt.Errorf("查询会话历史失败: %w", err)
		}
	} else {
		if err := s.db.WithContext(ctx).
			Where("session_id = ?", sessionID).
			Order("seq ASC").
			Find(&records).Error; err != nil {
			return nil, fmt.Errorf("查询会话历史失败: %w", err)
		}
	}

	msgs := make([]*schema.Message, 0, len(records))
	for _, rec := range records {
		msg := s.restoreMessage(&rec)
		if msg != nil {
			msgs = append(msgs, msg)
		}
	}

	if s.cache != nil && len(msgs) > 0 {
		go s.cache.Set(context.Background(), sessionID, msgs)
	}

	return msgs, nil
}

// restoreMessage 从结构化记录还原 Eino Message
func (s *MySQLSessionStore) restoreMessage(rec *model.SessionMessage) *schema.Message {
	if rec.MessageMeta != nil {
		var msg schema.Message
		if err := json.Unmarshal([]byte(*rec.MessageMeta), &msg); err == nil {
			return &msg
		}
	}

	msg := &schema.Message{Role: schema.RoleType(rec.Role)}
	if rec.Content != nil {
		msg.Content = *rec.Content
	}
	if rec.ToolName != "" {
		msg.ToolName = rec.ToolName
	}
	return msg
}

// Clear 清除会话全部消息及缓存
func (s *MySQLSessionStore) Clear(ctx context.Context, sessionID string) error {
	if err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&model.SessionMessage{}).Error; err != nil {
		return fmt.Errorf("清除会话消息失败: %w", err)
	}
	if s.cache != nil {
		s.cache.Del(ctx, sessionID)
	}
	return nil
}

// ============================================
// 会话元数据操作（v4 新增）
// ============================================

func (s *MySQLSessionStore) CreateSession(ctx context.Context, meta *SessionMeta) error {
	rec := &model.SessionMeta{
		SessionID:    meta.SessionID,
		UserID:       meta.UserID,
		EmployeeID:   meta.EmployeeID,
		Role:         meta.Role,
		Status:       meta.Status,
		Summary:      meta.Summary,
		CheckpointID: meta.CheckpointID,
	}
	if rec.Status == "" {
		rec.Status = SessionStatusActive
	}
	return s.metaRepo.Create(ctx, rec)
}

func (s *MySQLSessionStore) GetSession(ctx context.Context, sessionID string) (*SessionMeta, error) {
	rec, err := s.metaRepo.GetBySessionID(ctx, sessionID)
	if err != nil || rec == nil {
		return nil, err
	}
	return &SessionMeta{
		SessionID:    rec.SessionID,
		UserID:       rec.UserID,
		EmployeeID:   rec.EmployeeID,
		Role:         rec.Role,
		Status:       rec.Status,
		Summary:      rec.Summary,
		CheckpointID: rec.CheckpointID,
		MessageCount: rec.MessageCount,
	}, nil
}

func (s *MySQLSessionStore) UpdateSession(ctx context.Context, sessionID string, updates map[string]any) error {
	return s.metaRepo.Update(ctx, sessionID, updates)
}

func (s *MySQLSessionStore) DeleteSession(ctx context.Context, sessionID string) error {
	if err := s.Clear(ctx, sessionID); err != nil {
		return err
	}
	return s.metaRepo.Delete(ctx, sessionID)
}

func (s *MySQLSessionStore) ListSessions(ctx context.Context, userID uint, status string) ([]*SessionMeta, error) {
	recs, err := s.metaRepo.List(ctx, userID, status)
	if err != nil {
		return nil, err
	}
	result := make([]*SessionMeta, 0, len(recs))
	for _, rec := range recs {
		result = append(result, &SessionMeta{
			SessionID:    rec.SessionID,
			UserID:       rec.UserID,
			EmployeeID:   rec.EmployeeID,
			Role:         rec.Role,
			Status:       rec.Status,
			Summary:      rec.Summary,
			CheckpointID: rec.CheckpointID,
			MessageCount: rec.MessageCount,
		})
	}
	return result, nil
}

// ============================================
// Checkpoint 状态快照（v4 保留，仅 Interrupt 恢复时使用）
// ============================================

func (s *MySQLSessionStore) SaveCheckpointState(ctx context.Context, sessionID string, key string, state any) error {
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
		return fmt.Errorf("保存Checkpoint状态[%s]失败: %w", key, err)
	}
	return nil
}

func (s *MySQLSessionStore) GetCheckpointState(ctx context.Context, sessionID string, key string, target any) (bool, error) {
	var record SessionState
	err := s.db.WithContext(ctx).Where("id = ?", sessionID+":"+key).First(&record).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, fmt.Errorf("查询Checkpoint状态[%s]失败: %w", key, err)
	}
	if err := json.Unmarshal([]byte(record.Data), target); err != nil {
		return false, fmt.Errorf("反序列化Checkpoint状态[%s]失败: %w", key, err)
	}
	return true, nil
}

func (s *MySQLSessionStore) DeleteCheckpointState(ctx context.Context, sessionID string, key string) error {
	return s.db.WithContext(ctx).
		Where("id = ?", sessionID+":"+key).
		Delete(&SessionState{}).Error
}

// ============================================
// v3 兼容别名（Phase 5 清理时移除）
// ============================================

func (s *MySQLSessionStore) SaveState(ctx context.Context, sessionID string, key string, state any) error {
	return s.SaveCheckpointState(ctx, sessionID, key, state)
}

func (s *MySQLSessionStore) GetState(ctx context.Context, sessionID string, key string, target any) (bool, error) {
	return s.GetCheckpointState(ctx, sessionID, key, target)
}

func (s *MySQLSessionStore) DeleteState(ctx context.Context, sessionID string, key string) error {
	return s.DeleteCheckpointState(ctx, sessionID, key)
}

// ============================================
// 辅助函数
// ============================================

func truncateContent(content string, maxLen int) string {
	runes := []rune(content)
	if len(runes) <= maxLen {
		return content
	}
	return string(runes[:maxLen]) + "..."
}

var _ SessionStore = (*MySQLSessionStore)(nil)

func (s *MySQLSessionStore) GetDB() *gorm.DB { return s.db }
