package crypto

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// RedisBlacklistOptions 定义 Redis 黑名单配置。
type RedisBlacklistOptions struct {
	KeyPrefix        string
	OperationTimeout time.Duration
}

// RedisTokenBlacklistStore 定义 Redis 黑名单实现。
type RedisTokenBlacklistStore struct {
	client   redis.UniversalClient
	fallback *InMemoryTokenBlacklistStore
	opts     RedisBlacklistOptions
}

// NewRedisTokenBlacklistStore 创建 Redis 黑名单实现。
func NewRedisTokenBlacklistStore(client redis.UniversalClient, opts RedisBlacklistOptions) *RedisTokenBlacklistStore {
	return &RedisTokenBlacklistStore{
		client:   client,
		fallback: NewInMemoryTokenBlacklistStore(),
		opts:     opts,
	}
}

// Add 加入黑名单；Redis 异常时退化到本地存储。
func (s *RedisTokenBlacklistStore) Add(token string, expireAt time.Time) {
	if token == "" {
		return
	}
	if s.client == nil {
		s.fallback.Add(token, expireAt)
		return
	}
	ttl := time.Until(expireAt)
	if ttl <= 0 {
		s.fallback.Add(token, expireAt)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.operationTimeout())
	defer cancel()
	if err := s.client.Set(ctx, s.redisKey(token), "1", ttl).Err(); err != nil {
		logger.S().Warn("redis 黑名单写入失败，回退到本地黑名单", "error", err, "token_hash", hashToken(token))
		s.fallback.Add(token, expireAt)
	}
}

// Contains 判断令牌是否在黑名单中；Redis 异常时回退到本地存储。
func (s *RedisTokenBlacklistStore) Contains(token string) bool {
	if token == "" {
		return false
	}
	if s.client == nil {
		return s.fallback.Contains(token)
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.operationTimeout())
	defer cancel()
	ok, err := s.client.Exists(ctx, s.redisKey(token)).Result()
	if err != nil {
		logger.S().Warn("redis 黑名单读取失败，回退到本地黑名单", "error", err, "token_hash", hashToken(token))
		return s.fallback.Contains(token)
	}
	if ok > 0 {
		return true
	}
	return s.fallback.Contains(token)
}

// Cleanup 清理本地降级黑名单。
func (s *RedisTokenBlacklistStore) Cleanup() {
	s.fallback.Cleanup()
}

// Close 关闭底层客户端。
func (s *RedisTokenBlacklistStore) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *RedisTokenBlacklistStore) redisKey(token string) string {
	return s.opts.KeyPrefix + "jwt:blacklist:" + hashToken(token)
}

func (s *RedisTokenBlacklistStore) operationTimeout() time.Duration {
	if s.opts.OperationTimeout <= 0 {
		return 2 * time.Second
	}
	return s.opts.OperationTimeout
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
