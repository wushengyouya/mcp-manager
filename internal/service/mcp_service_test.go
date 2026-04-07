package service

import (
	"context"
	"testing"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/database"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
	"github.com/stretchr/testify/require"
)

// setupMCPServiceTest 初始化服务管理测试依赖
func setupMCPServiceTest(t *testing.T) (repository.MCPServiceRepository, repository.ToolRepository, repository.AuditLogRepository, *mcpclient.Manager) {
	t.Helper()

	db, err := database.Init(config.DatabaseConfig{
		Driver:       "sqlite",
		DSN:          ":memory:",
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, database.Migrate(db))

	return repository.NewMCPServiceRepository(db), repository.NewToolRepository(db), repository.NewAuditLogRepository(db), mcpclient.NewManager(config.AppConfig{})
}

// TestMCPServiceDeleteSoftDeletesServiceAndTools 验证删除服务时会软删除工具并写入审计
func TestMCPServiceDeleteSoftDeletesServiceAndTools(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, auditRepo, manager := setupMCPServiceTest(t)
	runtimeAdapter := NewLocalRuntimeAdapter(manager)
	svc := NewMCPService(serviceRepo, toolRepo, runtimeAdapter, runtimeAdapter, NewDBAuditSink(auditRepo), nil)

	created, err := svc.Create(ctx, CreateMCPServiceInput{
		Name:          "streamhttp-test",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://127.0.0.1:28080/mcp",
	}, AuditEntry{UserID: "u-1", Username: "root"})
	require.NoError(t, err)

	require.NoError(t, toolRepo.Create(ctx, &entity.Tool{
		MCPServiceID: created.ID,
		Name:         "search",
		Description:  "tool",
		IsEnabled:    true,
	}))

	err = svc.Delete(ctx, created.ID, AuditEntry{UserID: "u-1", Username: "root"})
	require.NoError(t, err)

	_, err = serviceRepo.GetByID(ctx, created.ID)
	require.ErrorIs(t, err, repository.ErrNotFound)

	tools, err := toolRepo.ListByService(ctx, created.ID)
	require.NoError(t, err)
	require.Empty(t, tools)

	logs, total, err := auditRepo.List(ctx, repository.AuditListFilter{Action: "delete_service"})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	require.Equal(t, created.ID, logs[0].ResourceID)
	require.Equal(t, false, logs[0].Detail["auto_disconnected"])
	require.Equal(t, float64(1), logs[0].Detail["tool_soft_deleted_count"])
}

// TestMCPServiceCreateAllowsReusingNameAfterSoftDelete 验证软删除后可复用同名服务
func TestMCPServiceCreateAllowsReusingNameAfterSoftDelete(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, auditRepo, manager := setupMCPServiceTest(t)
	runtimeAdapter := NewLocalRuntimeAdapter(manager)
	svc := NewMCPService(serviceRepo, toolRepo, runtimeAdapter, runtimeAdapter, NewDBAuditSink(auditRepo), nil)

	first, err := svc.Create(ctx, CreateMCPServiceInput{
		Name:          "streamhttp-test",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://127.0.0.1:28080/mcp",
	}, AuditEntry{UserID: "u-1", Username: "root"})
	require.NoError(t, err)

	require.NoError(t, svc.Delete(ctx, first.ID, AuditEntry{UserID: "u-1", Username: "root"}))

	second, err := svc.Create(ctx, CreateMCPServiceInput{
		Name:          "streamhttp-test",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://127.0.0.1:28081/mcp",
	}, AuditEntry{UserID: "u-1", Username: "root"})
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
}

// TestMCPServiceCreateRejectsDuplicateActiveName 验证活动服务名称不能重复
func TestMCPServiceCreateRejectsDuplicateActiveName(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, _, manager := setupMCPServiceTest(t)
	runtimeAdapter := NewLocalRuntimeAdapter(manager)
	svc := NewMCPService(serviceRepo, toolRepo, runtimeAdapter, runtimeAdapter, nil, nil)

	_, err := svc.Create(ctx, CreateMCPServiceInput{
		Name:          "streamhttp-test",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://127.0.0.1:28080/mcp",
	}, AuditEntry{UserID: "u-1", Username: "root"})
	require.NoError(t, err)

	_, err = svc.Create(ctx, CreateMCPServiceInput{
		Name:          "streamhttp-test",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://127.0.0.1:28081/mcp",
	}, AuditEntry{UserID: "u-1", Username: "root"})
	require.Error(t, err)
	bizErr, ok := err.(*response.BizError)
	require.True(t, ok)
	require.Equal(t, 409, bizErr.HTTPStatus)
}

// TestToolRepositoryAllowsSameToolNameAcrossServices 验证不同服务可复用相同工具名
func TestToolRepositoryAllowsSameToolNameAcrossServices(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, _, _ := setupMCPServiceTest(t)

	firstService := &entity.MCPService{
		Name:          "svc-a",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://127.0.0.1:28080/mcp",
	}
	secondService := &entity.MCPService{
		Name:          "svc-b",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://127.0.0.1:28081/mcp",
	}
	require.NoError(t, serviceRepo.Create(ctx, firstService))
	require.NoError(t, serviceRepo.Create(ctx, secondService))

	require.NoError(t, toolRepo.Create(ctx, &entity.Tool{
		MCPServiceID: firstService.ID,
		Name:         "search",
		IsEnabled:    true,
	}))
	require.NoError(t, toolRepo.Create(ctx, &entity.Tool{
		MCPServiceID: secondService.ID,
		Name:         "search",
		IsEnabled:    true,
	}))
}
