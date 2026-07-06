package infra

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"github.com/CycleZero/Reimbee/model"
	"gorm.io/gorm"
)

// MySQLSessionStore MySQL 会话持久化实现
// 以 MySQL 为主存储，可选 Redis 缓存加速读取
type MySQLSessionStore struct {
	db    *gorm.DB            // GORM 数据库实例
	cache *RedisSessionCache  // 可选 Redis 缓存层（nil 表示不使用缓存）
}

// NewMySQLSessionStore 创建 MySQL 会话存储实例
// 自动执行 AutoMigrate 确保表结构存在
func NewMySQLSessionStore(data *Data, cache *RedisSessionCache) *MySQLSessionStore {
	store := &MySQLSessionStore{
		db:    data.DB,
		cache: cache,
	}

	// 自动迁移会话消息表
	if err := store.db.AutoMigrate(&model.SessionMessage{}); err != nil {
		// AutoMigrate 失败属于致命错误，应在启动阶段暴露
		panic(fmt.Errorf("会话消息表自动迁移失败: %w", err))
	}

	return store
}

// SaveMessages 保存一轮完整交互的消息到 MySQL，并异步更新缓存
func (s *MySQLSessionStore) SaveMessages(ctx context.Context, sessionID string, msgs []*schema.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	// 构建批量插入的记录
	records := make([]*model.SessionMessage, 0, len(msgs))
	for _, msg := range msgs {
		rawJSON, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("序列化消息失败: %w", err)
		}

		records = append(records, &model.SessionMessage{
			SessionID: sessionID,
			Role:      string(msg.Role),
			Content:   msg.Content,
			RawJSON:   string(rawJSON),
		})
	}

	// 同步批量写入 MySQL
	if err := s.db.WithContext(ctx).Create(records).Error; err != nil {
		return fmt.Errorf("保存会话消息失败: %w", err)
	}

	// 非阻塞更新 Redis 缓存
	if s.cache != nil {
		go func() {
			// 使用独立 context 避免父 context 取消影响缓存写入
			if err := s.cache.Set(context.Background(), sessionID, msgs); err != nil {
				// 缓存更新失败不影响主流程，仅静默忽略
			}
		}()
	}

	return nil
}

// GetHistory 获取会话历史消息（按时间正序）
// 优先从 Redis 缓存读取，未命中时回源 MySQL 并异步预热缓存
func (s *MySQLSessionStore) GetHistory(ctx context.Context, sessionID string, limit int) ([]*schema.Message, error) {
	// 尝试从 Redis 缓存读取
	if s.cache != nil {
		if msgs, ok := s.cache.Get(ctx, sessionID); ok {
			// 缓存命中，按 limit 截取最近的消息
			if limit > 0 && len(msgs) > limit {
				return msgs[len(msgs)-limit:], nil
			}
			return msgs, nil
		}
	}

	// 缓存未命中，从 MySQL 查询
	var records []model.SessionMessage

	if limit > 0 {
		// 使用子查询获取最近 N 条记录，再按时间正序排列
		subQuery := s.db.WithContext(ctx).
			Model(&model.SessionMessage{}).
			Select("id").
			Where("session_id = ?", sessionID).
			Order("created_at DESC").
			Limit(limit)

		if err := s.db.WithContext(ctx).
			Where("id IN (?)", subQuery).
			Order("created_at ASC").
			Find(&records).Error; err != nil {
			return nil, fmt.Errorf("查询会话历史失败: %w", err)
		}
	} else {
		// limit <= 0 表示获取全部消息
		if err := s.db.WithContext(ctx).
			Where("session_id = ?", sessionID).
			Order("created_at ASC").
			Find(&records).Error; err != nil {
			return nil, fmt.Errorf("查询会话历史失败: %w", err)
		}
	}

	// 反序列化 raw_json 为 schema.Message
	msgs := make([]*schema.Message, 0, len(records))
	for i := range records {
		var msg schema.Message
		if err := json.Unmarshal([]byte(records[i].RawJSON), &msg); err != nil {
			return nil, fmt.Errorf("反序列化消息失败(id=%d): %w", records[i].ID, err)
		}
		msgs = append(msgs, &msg)
	}

	// 非阻塞异步预热缓存
	if s.cache != nil && len(msgs) > 0 {
		go func() {
			s.cache.Set(context.Background(), sessionID, msgs)
		}()
	}

	return msgs, nil
}

// Clear 清除会话所有消息（MySQL + Redis 缓存）
func (s *MySQLSessionStore) Clear(ctx context.Context, sessionID string) error {
	// 删除 MySQL 中的全部记录
	if err := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&model.SessionMessage{}).Error; err != nil {
		return fmt.Errorf("清除会话消息失败: %w", err)
	}

	// 同步删除 Redis 缓存
	if s.cache != nil {
		if err := s.cache.Del(ctx, sessionID); err != nil {
			// 缓存删除失败不影响主流程，仅记录
		}
	}

	return nil
}

// 编译期接口检查：确保 MySQLSessionStore 实现 SessionStore 接口
var _ SessionStore = (*MySQLSessionStore)(nil)
