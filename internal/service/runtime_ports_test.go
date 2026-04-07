package service

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeRuntime struct {
	connectStatus  mcpclient.RuntimeStatus
	status         mcpclient.RuntimeStatus
	tools          []mcp.Tool
	toolResult     *mcp.CallToolResult
	connectErr     error
	listToolsErr   error
	callToolErr    error
	disconnectedID string
}

func (f *fakeRuntime) Connect(_ context.Context, _ *entity.MCPService) (mcpclient.RuntimeStatus, error) {
	return f.connectStatus, f.connectErr
}

func (f *fakeRuntime) Disconnect(_ context.Context, serviceID string) error {
	f.disconnectedID = serviceID
	return nil
}

func (f *fakeRuntime) GetStatus(string) (mcpclient.RuntimeStatus, bool) {
	if f.status.ServiceID == "" {
		return mcpclient.RuntimeStatus{}, false
	}
	return f.status, true
}

func (f *fakeRuntime) ListTools(context.Context, string) ([]mcp.Tool, mcpclient.RuntimeStatus, error) {
	return f.tools, f.status, f.listToolsErr
}

func (f *fakeRuntime) CallTool(context.Context, string, string, map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	return f.toolResult, f.status, f.callToolErr
}

func setupRuntimePortTest(t *testing.T) (repository.MCPServiceRepository, repository.ToolRepository, repository.RequestHistoryRepository) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&entity.MCPService{}, &entity.Tool{}, &entity.RequestHistory{}))
	return repository.NewMCPServiceRepository(db), repository.NewToolRepository(db), repository.NewRequestHistoryRepository(db)
}

func TestMCPServiceStatusUsesInjectedRuntimeReader(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, _ := setupRuntimePortTest(t)
	require.NoError(t, serviceRepo.Create(ctx, &entity.MCPService{
		Base:          entity.Base{ID: "svc-fake"},
		Name:          "svc-fake",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-fake.test/mcp",
		Status:        entity.ServiceStatusDisconnected,
	}))

	runtime := &fakeRuntime{
		status: mcpclient.RuntimeStatus{
			ServiceID:     "svc-fake",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
			FailureCount:  0,
		},
	}

	svc := NewMCPService(serviceRepo, toolRepo, runtime, runtime, NoopAuditSink{}, nil)
	status, err := svc.Status(ctx, "svc-fake")
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusConnected, status["status"])
	require.Equal(t, entity.ServiceStatusDisconnected, status["persisted_status"])
	require.Equal(t, entity.ServiceStatusConnected, status["runtime_status"])
	require.Equal(t, "runtime", status["status_source"])
}

func TestMCPServiceStatusFallsBackToPersistedWhenRuntimeMissing(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, _ := setupRuntimePortTest(t)
	require.NoError(t, serviceRepo.Create(ctx, &entity.MCPService{
		Base:          entity.Base{ID: "svc-persisted"},
		Name:          "svc-persisted",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-persisted.test/mcp",
		Status:        entity.ServiceStatusError,
		FailureCount:  3,
		LastError:     "boom",
	}))

	runtime := &fakeRuntime{}
	svc := NewMCPService(serviceRepo, toolRepo, runtime, runtime, NoopAuditSink{}, nil)
	status, err := svc.Status(ctx, "svc-persisted")
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusError, status["status"])
	require.Equal(t, entity.ServiceStatusError, status["persisted_status"])
	require.Nil(t, status["runtime_status"])
	require.Equal(t, "persisted", status["status_source"])
}

func TestToolServiceSyncWithInjectedCatalogExecutor(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, _ := setupRuntimePortTest(t)
	require.NoError(t, serviceRepo.Create(ctx, &entity.MCPService{
		Base:          entity.Base{ID: "svc-tools"},
		Name:          "svc-tools",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-tools.test/mcp",
		Status:        entity.ServiceStatusDisconnected,
	}))

	runtime := &fakeRuntime{
		status: mcpclient.RuntimeStatus{
			ServiceID:       "svc-tools",
			Status:          entity.ServiceStatusConnected,
			TransportType:   string(entity.TransportTypeStreamableHTTP),
			ProtocolVersion: "2025-03-26",
		},
		tools: []mcp.Tool{{Name: "search", Description: "search tool"}},
	}

	svc := NewToolService(toolRepo, serviceRepo, runtime, NoopAuditSink{})
	items, err := svc.Sync(ctx, "svc-tools", AuditEntry{Username: "tester"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "search", items[0].Name)
}

func TestToolInvokeServiceInvokeWithInjectedInvoker(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, historyRepo := setupRuntimePortTest(t)
	require.NoError(t, serviceRepo.Create(ctx, &entity.MCPService{
		Base:          entity.Base{ID: "svc-invoke"},
		Name:          "svc-invoke",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-invoke.test/mcp",
		Status:        entity.ServiceStatusConnected,
	}))
	require.NoError(t, toolRepo.Create(ctx, &entity.Tool{
		Base:         entity.Base{ID: "tool-invoke"},
		MCPServiceID: "svc-invoke",
		Name:         "echo",
		IsEnabled:    true,
	}))

	runtime := &fakeRuntime{
		status: mcpclient.RuntimeStatus{
			ServiceID:     "svc-invoke",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
		},
		toolResult: &mcp.CallToolResult{
			StructuredContent: map[string]any{"echo": "hello"},
			Content:           []mcp.Content{mcp.NewTextContent("hello")},
		},
	}

	svc := NewToolInvokeService(config.HistoryConfig{MaxBodyBytes: 4096, Compression: "none"}, toolRepo, serviceRepo, historyRepo, runtime)
	result, err := svc.Invoke(ctx, "tool-invoke", map[string]any{"text": "hello"}, AuditEntry{UserID: "u-1", Username: "tester"})
	require.NoError(t, err)
	require.Equal(t, string(entity.TransportTypeStreamableHTTP), result.Result["transport_type"])

	items, total, err := historyRepo.List(ctx, repository.HistoryListFilter{Page: 1, PageSize: 10, IsAdmin: true})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.WithinDuration(t, time.Now(), items[0].CreatedAt, time.Minute)
}
