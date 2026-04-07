package repository

import (
	"context"
	"testing"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMCPServiceRepositoryCreateGetAndUpdateStatus(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewMCPServiceRepository(db)

	service := &entity.MCPService{
		Name:          "svc-a",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-a.test/mcp",
		Tags:          entity.JSONStringList{"team-a", "prod"},
		Status:        entity.ServiceStatusDisconnected,
		Timeout:       30,
	}
	require.NoError(t, repo.Create(context.Background(), service))

	gotByID, err := repo.GetByID(context.Background(), service.ID)
	require.NoError(t, err)
	require.Equal(t, service.Name, gotByID.Name)

	gotByName, err := repo.GetByName(context.Background(), "svc-a")
	require.NoError(t, err)
	require.Equal(t, service.ID, gotByName.ID)

	require.NoError(t, repo.UpdateStatus(context.Background(), service.ID, entity.ServiceStatusError, 2, "boom"))
	gotByID, err = repo.GetByID(context.Background(), service.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusError, gotByID.Status)
	require.Equal(t, 2, gotByID.FailureCount)
	require.Equal(t, "boom", gotByID.LastError)
}

func TestMCPServiceRepositoryUniqueConflicts(t *testing.T) {
	runRepositoryMatrix(t, func(t *testing.T, db *gorm.DB) {
		repo := NewMCPServiceRepository(db)
		first := seedService(t, repo, "svc-a", entity.TransportTypeStreamableHTTP, "http://svc-a.test/mcp", []string{"team-a"})
		second := seedService(t, repo, "svc-b", entity.TransportTypeSSE, "http://svc-b.test/sse", []string{"team-b"})

		err := repo.Create(context.Background(), &entity.MCPService{
			Name:          first.Name,
			TransportType: entity.TransportTypeStreamableHTTP,
			URL:           "http://dup.test/mcp",
		})
		require.ErrorIs(t, err, ErrAlreadyExists)

		second.Name = first.Name
		err = repo.Update(context.Background(), second)
		require.ErrorIs(t, err, ErrAlreadyExists)
	})
}

func TestMCPServiceRepositoryDeleteAndNotFound(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewMCPServiceRepository(db)
	service := seedService(t, repo, "delete-svc", entity.TransportTypeStdio, "", []string{"cleanup"})

	require.NoError(t, repo.Delete(context.Background(), service.ID))
	_, err := repo.GetByID(context.Background(), service.ID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = repo.GetByName(context.Background(), service.Name)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, repo.Delete(context.Background(), service.ID), ErrNotFound)
}

func TestMCPServiceRepositoryListFiltersAndPagination(t *testing.T) {
	runRepositoryMatrix(t, func(t *testing.T, db *gorm.DB) {
		repo := NewMCPServiceRepository(db)
		seedService(t, repo, "svc-http-a", entity.TransportTypeStreamableHTTP, "http://a.test/mcp", []string{"alpha", "team-a"})
		seedService(t, repo, "svc-http-b", entity.TransportTypeStreamableHTTP, "http://b.test/mcp", []string{"beta"})
		seedService(t, repo, "svc-http-c", entity.TransportTypeStreamableHTTP, "http://c.test/mcp", []string{"alphabet"})
		seedService(t, repo, "svc-sse", entity.TransportTypeSSE, "http://sse.test/events", []string{"alpha"})

		items, total, err := repo.List(context.Background(), MCPServiceListFilter{
			Page:          1,
			PageSize:      5,
			TransportType: string(entity.TransportTypeStreamableHTTP),
			Tag:           "alpha",
		})
		require.NoError(t, err)
		require.Equal(t, int64(1), total)
		require.Len(t, items, 1)
		require.Equal(t, entity.TransportTypeStreamableHTTP, items[0].TransportType)
		require.Contains(t, []string(items[0].Tags), "alpha")
		require.NotContains(t, items[0].Name, "alphabet")
	})
}

func TestMCPServiceRepositoryResetConnectionStatuses(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewMCPServiceRepository(db)
	connected := seedService(t, repo, "svc-connected", entity.TransportTypeStreamableHTTP, "http://connected.test/mcp", []string{"alpha"})
	connecting := seedService(t, repo, "svc-connecting", entity.TransportTypeStreamableHTTP, "http://connecting.test/mcp", []string{"beta"})
	errored := seedService(t, repo, "svc-error", entity.TransportTypeStreamableHTTP, "http://error.test/mcp", []string{"gamma"})

	require.NoError(t, repo.UpdateStatus(context.Background(), connected.ID, entity.ServiceStatusConnected, 1, ""))
	require.NoError(t, repo.UpdateStatus(context.Background(), connecting.ID, entity.ServiceStatusConnecting, 2, "dialing"))
	require.NoError(t, repo.UpdateStatus(context.Background(), errored.ID, entity.ServiceStatusError, 3, "boom"))

	rows, err := repo.ResetConnectionStatuses(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(2), rows)

	gotConnected, err := repo.GetByID(context.Background(), connected.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusDisconnected, gotConnected.Status)
	require.Zero(t, gotConnected.FailureCount)
	require.Empty(t, gotConnected.LastError)

	gotConnecting, err := repo.GetByID(context.Background(), connecting.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusDisconnected, gotConnecting.Status)
	require.Zero(t, gotConnecting.FailureCount)
	require.Empty(t, gotConnecting.LastError)

	gotErrored, err := repo.GetByID(context.Background(), errored.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusError, gotErrored.Status)
	require.Equal(t, 3, gotErrored.FailureCount)
	require.Equal(t, "boom", gotErrored.LastError)
}
