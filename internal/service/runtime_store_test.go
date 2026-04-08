package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRedisRuntimeStore_SaveGetDelete(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	store := NewRedisRuntimeStore(client, RedisRuntimeStoreOptions{
		KeyPrefix:        "runtime-test:",
		SnapshotTTL:      time.Minute,
		OperationTimeout: time.Second,
	})

	now := time.Now().UTC()
	snapshot := mcpclient.RuntimeSnapshot{
		RuntimeStatus: mcpclient.RuntimeStatus{
			ServiceID:     "svc-1",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
			InFlight:      1,
			LastUsedAt:    &now,
		},
		ObservedAt: now,
	}

	require.NoError(t, store.SaveSnapshot(context.Background(), snapshot))

	got, ok, err := store.GetSnapshot(context.Background(), "svc-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.ServiceStatusConnected, got.Status)
	require.Equal(t, 1, got.InFlight)

	require.NoError(t, store.DeleteSnapshot(context.Background(), "svc-1"))
	_, ok, err = store.GetSnapshot(context.Background(), "svc-1")
	require.NoError(t, err)
	require.False(t, ok)
}
