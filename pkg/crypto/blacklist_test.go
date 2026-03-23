package crypto

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTokenBlacklist_AddContains 测试添加令牌后能正确查询到
func TestTokenBlacklist_AddContains(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		ttl     time.Duration
		wantHit bool
	}{
		{"valid_token", "tok-1", time.Hour, true},
		{"another_valid", "tok-2", 10 * time.Minute, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bl := NewTokenBlacklist()
			bl.Add(tt.token, time.Now().Add(tt.ttl))
			require.Equal(t, tt.wantHit, bl.Contains(tt.token))
		})
	}
}

// TestTokenBlacklist_UnknownToken 测试查询不存在的令牌返回 false
func TestTokenBlacklist_UnknownToken(t *testing.T) {
	bl := NewTokenBlacklist()
	bl.Add("existing", time.Now().Add(time.Hour))
	require.False(t, bl.Contains("unknown"))
}

// TestTokenBlacklist_ExpiredToken 测试过期令牌返回 false 并被自动清理
func TestTokenBlacklist_ExpiredToken(t *testing.T) {
	bl := NewTokenBlacklist()
	bl.Add("expired-tok", time.Now().Add(-time.Second))

	require.False(t, bl.Contains("expired-tok"), "expired token should not be found")

	bl.mu.RLock()
	_, exists := bl.items["expired-tok"]
	bl.mu.RUnlock()
	require.False(t, exists, "expired token should be auto-removed from map")
}

// TestTokenBlacklist_Cleanup 测试 Cleanup 移除过期令牌但保留有效令牌
func TestTokenBlacklist_Cleanup(t *testing.T) {
	bl := NewTokenBlacklist()
	bl.Add("active", time.Now().Add(time.Hour))
	bl.Add("expired-1", time.Now().Add(-time.Second))
	bl.Add("expired-2", time.Now().Add(-time.Minute))

	bl.Cleanup()

	bl.mu.RLock()
	count := len(bl.items)
	_, hasActive := bl.items["active"]
	_, hasExp1 := bl.items["expired-1"]
	_, hasExp2 := bl.items["expired-2"]
	bl.mu.RUnlock()

	require.Equal(t, 1, count)
	require.True(t, hasActive)
	require.False(t, hasExp1)
	require.False(t, hasExp2)
}

// TestTokenBlacklist_Concurrent 测试并发 Add 和 Contains 的安全性
func TestTokenBlacklist_Concurrent(t *testing.T) {
	bl := NewTokenBlacklist()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// 并发写入
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			bl.Add(fmt.Sprintf("tok-%d", id), time.Now().Add(time.Hour))
		}(i)
	}

	// 并发读取
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			bl.Contains(fmt.Sprintf("tok-%d", id))
		}(i)
	}

	wg.Wait()

	// 写入完成后所有令牌都应存在
	for i := range goroutines {
		require.True(t, bl.Contains(fmt.Sprintf("tok-%d", i)))
	}
}
