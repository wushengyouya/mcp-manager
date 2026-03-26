package repository

import (
	"context"
	"testing"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
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
	db := setupRepositoryTestDB(t)
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
	db := setupRepositoryTestDB(t)
	repo := NewMCPServiceRepository(db)
	seedService(t, repo, "svc-http-a", entity.TransportTypeStreamableHTTP, "http://a.test/mcp", []string{"alpha", "team-a"})
	seedService(t, repo, "svc-http-b", entity.TransportTypeStreamableHTTP, "http://b.test/mcp", []string{"beta"})
	seedService(t, repo, "svc-sse", entity.TransportTypeSSE, "http://sse.test/events", []string{"alpha"})

	items, total, err := repo.List(context.Background(), MCPServiceListFilter{
		Page:          1,
		PageSize:      1,
		TransportType: string(entity.TransportTypeStreamableHTTP),
		Tag:           "alpha",
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, entity.TransportTypeStreamableHTTP, items[0].TransportType)
	require.Contains(t, []string(items[0].Tags), "alpha")
}
