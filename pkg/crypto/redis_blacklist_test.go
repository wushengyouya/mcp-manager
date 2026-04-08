package crypto

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRedisTokenBlacklistStore_AddContains(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	store := NewRedisTokenBlacklistStore(client, RedisBlacklistOptions{
		KeyPrefix:        "test:",
		OperationTimeout: time.Second,
	})

	store.Add("token-a", time.Now().Add(time.Hour))
	require.True(t, store.Contains("token-a"))
	require.False(t, store.Contains("token-b"))
}

func TestRedisTokenBlacklistStore_FallsBackToLocalOnRedisFailure(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	store := NewRedisTokenBlacklistStore(client, RedisBlacklistOptions{
		KeyPrefix:        "test:",
		OperationTimeout: time.Second,
	})

	mini.Close()
	store.Add("token-fallback", time.Now().Add(time.Hour))
	require.True(t, store.Contains("token-fallback"))
}
