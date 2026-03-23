package crypto

import (
	"sync"
	"time"
)

// TokenBlacklist 定义进程内令牌黑名单
type TokenBlacklist struct {
	mu    sync.RWMutex
	items map[string]time.Time
}

// NewTokenBlacklist 创建黑名单
func NewTokenBlacklist() *TokenBlacklist {
	return &TokenBlacklist{items: make(map[string]time.Time)}
}

// Add 加入黑名单
func (b *TokenBlacklist) Add(token string, expireAt time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items[token] = expireAt
}

// Contains 判断令牌是否在黑名单中
func (b *TokenBlacklist) Contains(token string) bool {
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

// Cleanup 清理过期黑名单
func (b *TokenBlacklist) Cleanup() {
	now := time.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	for token, expireAt := range b.items {
		if now.After(expireAt) {
			delete(b.items, token)
		}
	}
}
