package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/schema"
)

// RedisSessionCache 会话消息 Redis 缓存层
// 为 MySQLSessionStore 提供热数据加速，缓存未命中时回源 MySQL
type RedisSessionCache struct {
	client *RedisClient // 包装的 Redis 客户端
	ttl    time.Duration // 缓存过期时间
}

// NewRedisSessionCache 创建 Redis 会话缓存实例（默认 TTL 5 分钟）
func NewRedisSessionCache(client *RedisClient) *RedisSessionCache {
	return &RedisSessionCache{
		client: client,
		ttl:    5 * time.Minute,
	}
}

// cacheKey 生成 Redis 缓存键
func (c *RedisSessionCache) cacheKey(sessionID string) string {
	return fmt.Sprintf("reimbee:cache:session:%s:messages", sessionID)
}

// Set 将消息列表写入 Redis 缓存
func (c *RedisSessionCache) Set(ctx context.Context, sessionID string, msgs []*schema.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	key := c.cacheKey(sessionID)
	pipe := c.client.Pipeline()

	pipe.Del(ctx, key)
	for i := 0; i < len(msgs); i++ {
		data, err := json.Marshal(msgs[i])
		if err != nil {
			return fmt.Errorf("序列化消息失败: %w", err)
		}
		pipe.RPush(ctx, key, string(data))
	}

	pipe.LTrim(ctx, key, -40, -1)
	pipe.Expire(ctx, key, c.ttl)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("缓存会话消息失败: %w", err)
	}

	return nil
}

// Get 从 Redis 缓存读取会话消息
// 返回值: 消息列表, 是否命中（false 表示未命中或缓存为空）
func (c *RedisSessionCache) Get(ctx context.Context, sessionID string) ([]*schema.Message, bool) {
	key := c.cacheKey(sessionID)
	results, err := c.client.LRange(ctx, key, 0, -1).Result()
	if err != nil || len(results) == 0 {
		return nil, false
	}

	msgs := make([]*schema.Message, 0, len(results))
	for _, raw := range results {
		var msg schema.Message
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			// 单条反序列化失败则整体返回未命中，触发回源 MySQL
			return nil, false
		}
		msgs = append(msgs, &msg)
	}

	return msgs, true
}

// Del 删除指定会话的 Redis 缓存
func (c *RedisSessionCache) Del(ctx context.Context, sessionID string) error {
	key := c.cacheKey(sessionID)
	return c.client.Del(ctx, key).Err()
}
