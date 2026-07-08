// Package infra 会话消息 Redis 缓存层
// 为 SessionRepo 提供热数据加速，缓存未命中时回源 MySQL
package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/CycleZero/blades"
)

// RedisSessionCache 会话消息 Redis 缓存
type RedisSessionCache struct {
	client *RedisClient
	ttl    time.Duration
}

// NewRedisSessionCache 创建缓存实例（默认 TTL 5 分钟）
func NewRedisSessionCache(client *RedisClient) *RedisSessionCache {
	return &RedisSessionCache{
		client: client,
		ttl:    5 * time.Minute,
	}
}

func (c *RedisSessionCache) cacheKey(sessionID string) string {
	return fmt.Sprintf("reimbee:cache:session:%s:messages", sessionID)
}

// Set 将 blades.Message 列表写入 Redis（DEL + RPUSH + LTRIM + EXPIRE）
func (c *RedisSessionCache) Set(ctx context.Context, sessionID string, msgs []*blades.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	key := c.cacheKey(sessionID)
	pipe := c.client.Pipeline()

	pipe.Del(ctx, key)
	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("序列化消息失败: %w", err)
		}
		pipe.RPush(ctx, key, string(data))
	}
	pipe.LTrim(ctx, key, -40, -1)
	pipe.Expire(ctx, key, c.ttl)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("缓存消息失败: %w", err)
	}
	return nil
}

// Get 从 Redis 读取 blades.Message 列表
// 返回 (nil, false) 表示未命中
func (c *RedisSessionCache) Get(ctx context.Context, sessionID string) ([]*blades.Message, bool) {
	key := c.cacheKey(sessionID)
	results, err := c.client.LRange(ctx, key, 0, -1).Result()
	if err != nil || len(results) == 0 {
		return nil, false
	}

	msgs := make([]*blades.Message, 0, len(results))
	for _, raw := range results {
		var msg blades.Message
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			return nil, false
		}
		msgs = append(msgs, &msg)
	}
	return msgs, true
}

// Del 删除缓存
func (c *RedisSessionCache) Del(ctx context.Context, sessionID string) error {
	return c.client.Del(ctx, c.cacheKey(sessionID)).Err()
}
