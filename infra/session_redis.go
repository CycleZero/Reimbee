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
// 使用 Pipeline 批量写入，保留最近 40 条消息（约 20 轮对话）
func (c *RedisSessionCache) Set(ctx context.Context, sessionID string, msgs []*schema.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	key := c.cacheKey(sessionID)
	pipe := c.client.Pipeline()

	// 从末尾遍历并 LPUSH，确保列表头部为最早的消息
	// LPUSH 后列表顺序: [msg[0], msg[1], ..., msg[n-1]]
	for i := len(msgs) - 1; i >= 0; i-- {
		data, err := json.Marshal(msgs[i])
		if err != nil {
			return fmt.Errorf("序列化消息失败: %w", err)
		}
		pipe.LPush(ctx, key, string(data))
	}

	// 保留最近 40 条消息
	pipe.LTrim(ctx, key, 0, 39)
	// 设置过期时间
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
