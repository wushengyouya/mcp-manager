package service

import (
	"context"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

type historySinkRecorder struct {
	items []*entity.RequestHistory
}

func (r *historySinkRecorder) Record(_ context.Context, item *entity.RequestHistory) error {
	cloned := *item
	r.items = append(r.items, &cloned)
	return nil
}

type asyncAuditRecorder struct {
	entries []AuditEntry
}

func (r *asyncAuditRecorder) Record(_ context.Context, entry AuditEntry) error {
	r.entries = append(r.entries, entry)
	return nil
}

func TestAsyncHistorySinkDrainsQueuedItemsOnStop(t *testing.T) {
	recorder := &historySinkRecorder{}
	sink := NewAsyncHistorySink(recorder, 1, 8)

	for i := 0; i < 3; i++ {
		require.NoError(t, sink.Record(context.Background(), &entity.RequestHistory{ID: time.Now().Add(time.Duration(i) * time.Millisecond).Format(time.RFC3339Nano)}))
	}
	require.NoError(t, sink.Stop(context.Background()))
	require.Len(t, recorder.items, 3)
}

func TestAsyncAuditSinkFallsBackToSyncWhenQueueIsFull(t *testing.T) {
	recorder := &asyncAuditRecorder{}
	sink := NewAsyncAuditSink(recorder, 1, 1)

	blocked := make(chan struct{})
	require.NoError(t, sink.queue.Submit(func(context.Context) error {
		<-blocked
		return nil
	}))
	require.NoError(t, sink.Record(context.Background(), AuditEntry{UserID: "u-1", Username: "tester"}))
	close(blocked)
	require.NoError(t, sink.Stop(context.Background()))
	require.Len(t, recorder.entries, 1)
	require.Equal(t, "u-1", recorder.entries[0].UserID)
}
