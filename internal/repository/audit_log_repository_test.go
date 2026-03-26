package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuditLogRepositoryListFiltersAndPagination(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewAuditLogRepository(db)
	base := time.Now().UTC().Add(-2 * time.Hour)
	seedAuditLog(t, repo, "user-1", "login", "auth", base)
	target := seedAuditLog(t, repo, "user-1", "sync_tools", "mcp_service", base.Add(time.Hour))
	seedAuditLog(t, repo, "user-2", "delete_service", "mcp_service", base.Add(2*time.Hour))

	start := base.Add(30 * time.Minute)
	end := base.Add(90 * time.Minute)
	items, total, err := repo.List(context.Background(), AuditListFilter{
		Page:         1,
		PageSize:     10,
		UserID:       "user-1",
		Action:       "sync_tools",
		ResourceType: "mcp_service",
		StartAt:      &start,
		EndAt:        &end,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, target.ID, items[0].ID)

	items, total, err = repo.List(context.Background(), AuditListFilter{Page: 1, PageSize: 1, ResourceType: "mcp_service"})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, items, 1)
}

func TestAuditLogRepositoryDeleteOlderThan(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewAuditLogRepository(db)
	base := time.Now().UTC().Add(-2 * time.Hour)
	seedAuditLog(t, repo, "user-1", "login", "auth", base)
	seedAuditLog(t, repo, "user-2", "sync_tools", "mcp_service", base.Add(time.Hour))

	rows, err := repo.DeleteOlderThan(context.Background(), base.Add(30*time.Minute))
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	items, total, err := repo.List(context.Background(), AuditListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, "sync_tools", items[0].Action)
}
