package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/database"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/tests/pgtest"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.Init(sqliteRepositoryCfg())
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	t.Cleanup(func() {
		_ = database.Close()
	})
	return db
}

func sqliteRepositoryCfg() config.DatabaseConfig {
	return config.DatabaseConfig{
		Driver:          "sqlite",
		DSN:             ":memory:",
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
}

func setupRepositoryTestDBWithConfig(t *testing.T, cfg config.DatabaseConfig) *gorm.DB {
	t.Helper()

	db, err := database.Init(cfg)
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	t.Cleanup(func() {
		_ = database.Close()
	})
	return db
}

func runRepositoryMatrix(t *testing.T, fn func(t *testing.T, db *gorm.DB)) {
	t.Helper()

	t.Run("sqlite", func(t *testing.T) {
		fn(t, setupRepositoryTestDBWithConfig(t, sqliteRepositoryCfg()))
	})

	t.Run("postgres", func(t *testing.T) {
		fn(t, setupRepositoryTestDBWithConfig(t, pgtest.NewPostgresDatabaseConfig(t)))
	})
}

func seedUser(t *testing.T, repo UserRepository, username, email string, role entity.Role, active bool) *entity.User {
	t.Helper()
	user := &entity.User{
		Username:     username,
		Password:     "hashed-password",
		Email:        email,
		Role:         role,
		IsActive:     active,
		IsFirstLogin: true,
	}
	require.NoError(t, repo.Create(context.Background(), user))
	return user
}

func seedService(t *testing.T, repo MCPServiceRepository, name string, transport entity.TransportType, url string, tags []string) *entity.MCPService {
	t.Helper()
	service := &entity.MCPService{
		Name:          name,
		TransportType: transport,
		URL:           url,
		Command:       "cmd-" + name,
		Tags:          tags,
		Status:        entity.ServiceStatusDisconnected,
		Timeout:       30,
	}
	require.NoError(t, repo.Create(context.Background(), service))
	return service
}

func seedTool(t *testing.T, repo ToolRepository, serviceID, name, desc string) *entity.Tool {
	t.Helper()
	item := &entity.Tool{
		MCPServiceID: serviceID,
		Name:         name,
		Description:  desc,
		InputSchema:  entity.JSONMap{"type": "object"},
		IsEnabled:    true,
		SyncedAt:     time.Now(),
	}
	require.NoError(t, repo.Create(context.Background(), item))
	return item
}

func seedHistory(t *testing.T, repo RequestHistoryRepository, serviceID, toolName, userID string, status entity.RequestStatus, createdAt time.Time) *entity.RequestHistory {
	t.Helper()
	item := &entity.RequestHistory{
		ID:              uuid.NewString(),
		MCPServiceID:    serviceID,
		ToolName:        toolName,
		UserID:          userID,
		RequestBody:     entity.JSONMap{"q": "hello"},
		ResponseBody:    entity.JSONMap{"ok": true},
		CompressionType: "none",
		Status:          status,
		DurationMS:      10,
		CreatedAt:       createdAt,
	}
	require.NoError(t, repo.Create(context.Background(), item))
	return item
}

func seedAuditLog(t *testing.T, repo AuditLogRepository, userID, action, resourceType string, createdAt time.Time) *entity.AuditLog {
	t.Helper()
	item := &entity.AuditLog{
		ID:           uuid.NewString(),
		UserID:       userID,
		Username:     userID + "-name",
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   uuid.NewString(),
		Detail:       entity.JSONMap{"ok": true},
		CreatedAt:    createdAt,
	}
	require.NoError(t, repo.Create(context.Background(), item))
	return item
}
