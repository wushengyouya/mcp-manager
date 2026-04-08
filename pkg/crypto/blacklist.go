package crypto

import (
	"sync"
	"time"
)

// TokenBlacklistStore 定义令牌黑名单存储契约。
type TokenBlacklistStore interface {
	Add(token string, expireAt time.Time)
	Contains(token string) bool
	Cleanup()
	Close() error
}

// InMemoryTokenBlacklistStore 定义进程内令牌黑名单。
type InMemoryTokenBlacklistStore struct {
	mu    sync.RWMutex
	items map[string]time.Time
}

// NewInMemoryTokenBlacklistStore 创建进程内黑名单。
func NewInMemoryTokenBlacklistStore() *InMemoryTokenBlacklistStore {
	return &InMemoryTokenBlacklistStore{items: make(map[string]time.Time)}
}

// NewTokenBlacklist 为旧调用方保留兼容入口。
func NewTokenBlacklist() *InMemoryTokenBlacklistStore {
	return NewInMemoryTokenBlacklistStore()
}

// Add 加入黑名单。
func (b *InMemoryTokenBlacklistStore) Add(token string, expireAt time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items[token] = expireAt
}

// Contains 判断令牌是否在黑名单中。
func (b *InMemoryTokenBlacklistStore) Contains(token string) bool {
	b.mu.RLock()
	expireAt, ok := b.items[token]
	b.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(expireAt) {
		b.mu.Lock()
		delete(b.items, token)
		b.mu.Unlock()
		return false
	}
	return true
}

// Cleanup 清理过期黑名单。
func (b *InMemoryTokenBlacklistStore) Cleanup() {
	now := time.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	for token, expireAt := range b.items {
		if now.After(expireAt) {
			delete(b.items, token)
		}
	}
}

// Close 关闭黑名单存储。
func (b *InMemoryTokenBlacklistStore) Close() error {
	return nil
}
