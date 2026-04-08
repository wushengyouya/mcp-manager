package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// RedisRuntimeStoreOptions 定义 Redis 运行态快照配置。
type RedisRuntimeStoreOptions struct {
	KeyPrefix        string
	SnapshotTTL      time.Duration
	OperationTimeout time.Duration
}

// NoopRuntimeStore 定义空实现。
type NoopRuntimeStore struct{}

// SaveSnapshot 保存运行态快照。
func (NoopRuntimeStore) SaveSnapshot(context.Context, mcpclient.RuntimeSnapshot) error { return nil }

// GetSnapshot 读取运行态快照。
func (NoopRuntimeStore) GetSnapshot(context.Context, string) (mcpclient.RuntimeSnapshot, bool, error) {
	return mcpclient.RuntimeSnapshot{}, false, nil
}

// DeleteSnapshot 删除运行态快照。
func (NoopRuntimeStore) DeleteSnapshot(context.Context, string) error { return nil }

// MemoryRuntimeStore 定义内存快照实现，主要用于测试。
type MemoryRuntimeStore struct {
	mu    sync.RWMutex
	items map[string]mcpclient.RuntimeSnapshot
}

// NewMemoryRuntimeStore 创建内存快照实现。
func NewMemoryRuntimeStore() *MemoryRuntimeStore {
	return &MemoryRuntimeStore{items: make(map[string]mcpclient.RuntimeSnapshot)}
}

// SaveSnapshot 保存运行态快照。
func (s *MemoryRuntimeStore) SaveSnapshot(_ context.Context, snapshot mcpclient.RuntimeSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[snapshot.ServiceID] = snapshot
	return nil
}

// GetSnapshot 读取运行态快照。
func (s *MemoryRuntimeStore) GetSnapshot(_ context.Context, serviceID string) (mcpclient.RuntimeSnapshot, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot, ok := s.items[serviceID]
	return snapshot, ok, nil
}

// DeleteSnapshot 删除运行态快照。
func (s *MemoryRuntimeStore) DeleteSnapshot(_ context.Context, serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, serviceID)
	return nil
}

// RedisRuntimeStore 定义 Redis 快照实现。
type RedisRuntimeStore struct {
	client redis.UniversalClient
	opts   RedisRuntimeStoreOptions
}

// NewRedisRuntimeStore 创建 Redis 快照实现。
func NewRedisRuntimeStore(client redis.UniversalClient, opts RedisRuntimeStoreOptions) *RedisRuntimeStore {
	return &RedisRuntimeStore{client: client, opts: opts}
}

// SaveSnapshot 保存运行态快照。
func (s *RedisRuntimeStore) SaveSnapshot(ctx context.Context, snapshot mcpclient.RuntimeSnapshot) error {
	if s.client == nil {
		return errors.New("runtime snapshot redis client is nil")
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return s.client.Set(ctx, s.redisKey(snapshot.ServiceID), payload, s.snapshotTTL()).Err()
}

// GetSnapshot 读取运行态快照。
func (s *RedisRuntimeStore) GetSnapshot(ctx context.Context, serviceID string) (mcpclient.RuntimeSnapshot, bool, error) {
	if s.client == nil {
		return mcpclient.RuntimeSnapshot{}, false, errors.New("runtime snapshot redis client is nil")
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	payload, err := s.client.Get(ctx, s.redisKey(serviceID)).Bytes()
	if err == redis.Nil {
		return mcpclient.RuntimeSnapshot{}, false, nil
	}
	if err != nil {
		return mcpclient.RuntimeSnapshot{}, false, err
	}
	var snapshot mcpclient.RuntimeSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return mcpclient.RuntimeSnapshot{}, false, err
	}
	return snapshot, true, nil
}

// DeleteSnapshot 删除运行态快照。
func (s *RedisRuntimeStore) DeleteSnapshot(ctx context.Context, serviceID string) error {
	if s.client == nil {
		return errors.New("runtime snapshot redis client is nil")
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	return s.client.Del(ctx, s.redisKey(serviceID)).Err()
}

func (s *RedisRuntimeStore) redisKey(serviceID string) string {
	return s.opts.KeyPrefix + "runtime:snapshot:" + serviceID
}

func (s *RedisRuntimeStore) snapshotTTL() time.Duration {
	if s.opts.SnapshotTTL <= 0 {
		return 30 * time.Second
	}
	return s.opts.SnapshotTTL
}

func (s *RedisRuntimeStore) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.opts.OperationTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, s.opts.OperationTimeout)
}

func logRuntimeSnapshotError(action, serviceID string, err error) {
	logger.S().Warn("运行态快照操作失败", "action", action, "service_id", serviceID, "error", err)
}
