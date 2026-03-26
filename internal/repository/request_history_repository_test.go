package repository

import (
	"context"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

func TestRequestHistoryRepositoryGetByIDAndNotFound(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewRequestHistoryRepository(db)
	item := seedHistory(t, repo, "svc-1", "search", "user-1", entity.RequestStatusSuccess, time.Now())

	got, err := repo.GetByID(context.Background(), item.ID)
	require.NoError(t, err)
	require.Equal(t, item.ID, got.ID)

	_, err = repo.GetByID(context.Background(), "missing")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRequestHistoryRepositoryListFiltersForAdminAndUser(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewRequestHistoryRepository(db)
	base := time.Now().UTC().Add(-3 * time.Hour)
	seedHistory(t, repo, "svc-1", "search", "user-1", entity.RequestStatusSuccess, base)
	target := seedHistory(t, repo, "svc-1", "search", "user-1", entity.RequestStatusFailed, base.Add(time.Hour))
	seedHistory(t, repo, "svc-2", "lookup", "user-2", entity.RequestStatusFailed, base.Add(2*time.Hour))

	start := base.Add(30 * time.Minute)
	end := base.Add(90 * time.Minute)
	items, total, err := repo.List(context.Background(), HistoryListFilter{
		Page:      1,
		PageSize:  10,
		ServiceID: "svc-1",
		ToolName:  "search",
		Status:    string(entity.RequestStatusFailed),
		UserID:    "user-1",
		IsAdmin:   false,
		StartAt:   &start,
		EndAt:     &end,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, target.ID, items[0].ID)

	items, total, err = repo.List(context.Background(), HistoryListFilter{
		Page:     1,
		PageSize: 1,
		UserID:   "ignored",
		IsAdmin:  true,
		Status:   string(entity.RequestStatusFailed),
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, items, 1)
}
