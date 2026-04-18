package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// AuthStateStoreOptions 定义认证状态缓存配置。
type AuthStateStoreOptions struct {
	KeyPrefix        string
	OperationTimeout time.Duration
}

// UserTokenVersionStore 定义用户 token_version 缓存接口。
type UserTokenVersionStore interface {
	GetUserTokenVersion(ctx context.Context, userID string) (int64, bool, error)
	SetUserTokenVersion(ctx context.Context, userID string, version int64) error
	DeleteUserTokenVersion(ctx context.Context, userID string) error
}

// SessionStateStore 定义会话状态缓存接口。
type SessionStateStore interface {
	GetSessionStatus(ctx context.Context, sessionID string) (entity.AuthSessionStatus, bool, error)
	SetSessionStatus(ctx context.Context, sessionID string, status entity.AuthSessionStatus, ttl time.Duration) error
	DeleteSessionStatus(ctx context.Context, sessionID string) error
}

// NoopUserTokenVersionStore 定义空实现。
type NoopUserTokenVersionStore struct{}

func (NoopUserTokenVersionStore) GetUserTokenVersion(context.Context, string) (int64, bool, error) {
	return 0, false, nil
}
func (NoopUserTokenVersionStore) SetUserTokenVersion(context.Context, string, int64) error {
	return nil
}
func (NoopUserTokenVersionStore) DeleteUserTokenVersion(context.Context, string) error { return nil }

// NoopSessionStateStore 定义空实现。
type NoopSessionStateStore struct{}

func (NoopSessionStateStore) GetSessionStatus(context.Context, string) (entity.AuthSessionStatus, bool, error) {
	return "", false, nil
}
func (NoopSessionStateStore) SetSessionStatus(context.Context, string, entity.AuthSessionStatus, time.Duration) error {
	return nil
}
func (NoopSessionStateStore) DeleteSessionStatus(context.Context, string) error { return nil }

// RedisUserTokenVersionStore 定义 Redis 用户版本缓存实现。
type RedisUserTokenVersionStore struct {
	client redis.UniversalClient
	opts   AuthStateStoreOptions
}

// NewRedisUserTokenVersionStore 创建 Redis 用户版本缓存实现。
func NewRedisUserTokenVersionStore(client redis.UniversalClient, opts AuthStateStoreOptions) *RedisUserTokenVersionStore {
	return &RedisUserTokenVersionStore{client: client, opts: opts}
}

func (s *RedisUserTokenVersionStore) GetUserTokenVersion(ctx context.Context, userID string) (int64, bool, error) {
	if s.client == nil || userID == "" {
		return 0, false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.operationTimeout())
	defer cancel()
	value, err := s.client.Get(ctx, s.userKey(userID)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false, err
	}
	return version, true, nil
}

func (s *RedisUserTokenVersionStore) SetUserTokenVersion(ctx context.Context, userID string, version int64) error {
	if s.client == nil || userID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.operationTimeout())
	defer cancel()
	return s.client.Set(ctx, s.userKey(userID), strconv.FormatInt(version, 10), 0).Err()
}

func (s *RedisUserTokenVersionStore) DeleteUserTokenVersion(ctx context.Context, userID string) error {
	if s.client == nil || userID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.operationTimeout())
	defer cancel()
	return s.client.Del(ctx, s.userKey(userID)).Err()
}

func (s *RedisUserTokenVersionStore) userKey(userID string) string {
	return s.opts.KeyPrefix + "auth:user-version:" + userID
}

func (s *RedisUserTokenVersionStore) operationTimeout() time.Duration {
	if s.opts.OperationTimeout <= 0 {
		return 2 * time.Second
	}
	return s.opts.OperationTimeout
}

// RedisSessionStateStore 定义 Redis 会话状态缓存实现。
type RedisSessionStateStore struct {
	client redis.UniversalClient
	opts   AuthStateStoreOptions
}

// NewRedisSessionStateStore 创建 Redis 会话状态缓存实现。
func NewRedisSessionStateStore(client redis.UniversalClient, opts AuthStateStoreOptions) *RedisSessionStateStore {
	return &RedisSessionStateStore{client: client, opts: opts}
}

func (s *RedisSessionStateStore) GetSessionStatus(ctx context.Context, sessionID string) (entity.AuthSessionStatus, bool, error) {
	if s.client == nil || sessionID == "" {
		return "", false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.operationTimeout())
	defer cancel()
	value, err := s.client.Get(ctx, s.sessionKey(sessionID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return entity.AuthSessionStatus(value), true, nil
}

func (s *RedisSessionStateStore) SetSessionStatus(ctx context.Context, sessionID string, status entity.AuthSessionStatus, ttl time.Duration) error {
	if s.client == nil || sessionID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.operationTimeout())
	defer cancel()
	if ttl <= 0 {
		ttl = time.Minute
	}
	return s.client.Set(ctx, s.sessionKey(sessionID), string(status), ttl).Err()
}

func (s *RedisSessionStateStore) DeleteSessionStatus(ctx context.Context, sessionID string) error {
	if s.client == nil || sessionID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.operationTimeout())
	defer cancel()
	return s.client.Del(ctx, s.sessionKey(sessionID)).Err()
}

func (s *RedisSessionStateStore) sessionKey(sessionID string) string {
	return s.opts.KeyPrefix + "auth:session-status:" + sessionID
}

func (s *RedisSessionStateStore) operationTimeout() time.Duration {
	if s.opts.OperationTimeout <= 0 {
		return 2 * time.Second
	}
	return s.opts.OperationTimeout
}

func cacheSetSessionStatus(ctx context.Context, store SessionStateStore, sessionID string, status entity.AuthSessionStatus, expireAt time.Time) {
	if store == nil || sessionID == "" {
		return
	}
	ttl := time.Until(expireAt)
	if ttl <= 0 {
		ttl = time.Minute
	}
	if err := store.SetSessionStatus(ctx, sessionID, status, ttl); err != nil {
		logger.S().Warn("写入会话状态缓存失败", "session_id", sessionID, "status", status, "error", err)
	}
}
