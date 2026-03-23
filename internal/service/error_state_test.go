package service

import (
	"context"
	"testing"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupErrorStateTest 初始化错误状态相关测试依赖
func setupErrorStateTest(t *testing.T) (*gorm.DB, repository.MCPServiceRepository, repository.ToolRepository, repository.RequestHistoryRepository, *mcpclient.Manager) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&entity.MCPService{}, &entity.Tool{}, &entity.RequestHistory{}))

	return db, repository.NewMCPServiceRepository(db), repository.NewToolRepository(db), repository.NewRequestHistoryRepository(db), mcpclient.NewManager(config.AppConfig{})
}

// TestToolServiceSyncRejectsErrorService 验证错误态服务不允许同步工具
func TestToolServiceSyncRejectsErrorService(t *testing.T) {
	_, serviceRepo, toolRepo, _, manager := setupErrorStateTest(t)
	ctx := context.Background()

	serviceItem := &entity.MCPService{
		Name:          "broken-svc",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://127.0.0.1:28080/mcp",
		Status:        entity.ServiceStatusError,
	}
	require.NoError(t, serviceRepo.Create(ctx, serviceItem))

	svc := NewToolService(toolRepo, serviceRepo, manager, NoopAuditSink{})
	_, err := svc.Sync(ctx, serviceItem.ID, AuditEntry{})
	require.Error(t, err)

	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeConflict, bizErr.Code)
	require.Equal(t, "服务处于错误状态，请先恢复连接", bizErr.Message)
}

// TestToolInvokeServiceRejectsErrorService 验证错误态服务不允许调用工具
func TestToolInvokeServiceRejectsErrorService(t *testing.T) {
	db, serviceRepo, toolRepo, historyRepo, manager := setupErrorStateTest(t)
	ctx := context.Background()

	serviceItem := &entity.MCPService{
		Name:          "broken-svc",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://127.0.0.1:28080/mcp",
		Status:        entity.ServiceStatusError,
	}
	require.NoError(t, serviceRepo.Create(ctx, serviceItem))

	toolItem := &entity.Tool{
		MCPServiceID: serviceItem.ID,
		Name:         "echo",
		Description:  "echo",
		InputSchema:  entity.JSONMap{"type": "object"},
		IsEnabled:    true,
	}
	require.NoError(t, toolRepo.Create(ctx, toolItem))

	svc := NewToolInvokeService(config.HistoryConfig{MaxBodyBytes: 8192, Compression: "none"}, toolRepo, serviceRepo, historyRepo, manager)
	_, err := svc.Invoke(ctx, toolItem.ID, map[string]any{"text": "hello"}, AuditEntry{UserID: "u-1", Username: "tester"})
	require.Error(t, err)

	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeConflict, bizErr.Code)
	require.Equal(t, "服务处于错误状态，请先恢复连接", bizErr.Message)

	var count int64
	require.NoError(t, db.Model(&entity.RequestHistory{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}
