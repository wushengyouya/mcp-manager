package repository

import (
	"context"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

func TestToolRepositoryGettersAndListByService(t *testing.T) {
	db := setupRepositoryTestDB(t)
	serviceRepo := NewMCPServiceRepository(db)
	repo := NewToolRepository(db)
	service := seedService(t, serviceRepo, "svc-tools", entity.TransportTypeStreamableHTTP, "http://svc.test/mcp", []string{"tools"})
	toolB := seedTool(t, repo, service.ID, "beta", "beta tool")
	seedTool(t, repo, service.ID, "alpha", "alpha tool")

	gotByID, err := repo.GetByID(context.Background(), toolB.ID)
	require.NoError(t, err)
	require.Equal(t, "beta", gotByID.Name)

	gotByName, err := repo.GetByServiceAndName(context.Background(), service.ID, "alpha")
	require.NoError(t, err)
	require.Equal(t, "alpha", gotByName.Name)

	items, err := repo.ListByService(context.Background(), service.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "alpha", items[0].Name)
	require.Equal(t, "beta", items[1].Name)
}

func TestToolRepositoryUpdateDeleteAndNotFound(t *testing.T) {
	db := setupRepositoryTestDB(t)
	serviceRepo := NewMCPServiceRepository(db)
	repo := NewToolRepository(db)
	service := seedService(t, serviceRepo, "svc-delete-tools", entity.TransportTypeStdio, "", []string{"local"})
	tool := seedTool(t, repo, service.ID, "search", "old description")

	tool.Description = "new description"
	require.NoError(t, repo.Update(context.Background(), tool))

	got, err := repo.GetByID(context.Background(), tool.ID)
	require.NoError(t, err)
	require.Equal(t, "new description", got.Description)

	rows, err := repo.DeleteByService(context.Background(), service.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)

	_, err = repo.GetByID(context.Background(), tool.ID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = repo.GetByServiceAndName(context.Background(), service.ID, "search")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestToolRepositoryBatchUpsertCreatesAndUpdates(t *testing.T) {
	db := setupRepositoryTestDB(t)
	serviceRepo := NewMCPServiceRepository(db)
	repo := NewToolRepository(db)
	service := seedService(t, serviceRepo, "svc-sync", entity.TransportTypeStreamableHTTP, "http://svc-sync.test/mcp", []string{"sync"})
	firstSync := time.Now().Add(-time.Hour)
	secondSync := time.Now().UTC().Truncate(time.Second)

	require.NoError(t, repo.BatchUpsert(context.Background(), []entity.Tool{
		{
			MCPServiceID: service.ID,
			Name:         "search",
			Description:  "search v1",
			InputSchema:  entity.JSONMap{"v": float64(1)},
			IsEnabled:    true,
			SyncedAt:     firstSync,
		},
	}))

	require.NoError(t, repo.BatchUpsert(context.Background(), []entity.Tool{
		{
			MCPServiceID: service.ID,
			Name:         "search",
			Description:  "search v2",
			InputSchema:  entity.JSONMap{"v": float64(2)},
			IsEnabled:    false,
			SyncedAt:     secondSync,
		},
		{
			MCPServiceID: service.ID,
			Name:         "lookup",
			Description:  "lookup v1",
			InputSchema:  entity.JSONMap{"kind": "lookup"},
			IsEnabled:    true,
			SyncedAt:     secondSync,
		},
	}))

	search, err := repo.GetByServiceAndName(context.Background(), service.ID, "search")
	require.NoError(t, err)
	require.Equal(t, "search v2", search.Description)
	require.Equal(t, entity.JSONMap{"v": float64(2)}, search.InputSchema)
	require.False(t, search.IsEnabled)
	require.WithinDuration(t, secondSync, search.SyncedAt, time.Second)

	items, err := repo.ListByService(context.Background(), service.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)
}
