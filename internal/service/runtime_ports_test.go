package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/database"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
	"github.com/stretchr/testify/require"
)

func requireDurationStringInRange(t *testing.T, value any, min time.Duration, max time.Duration) {
	t.Helper()
	s, ok := value.(string)
	require.True(t, ok)
	d, err := time.ParseDuration(s)
	require.NoError(t, err)
	require.GreaterOrEqual(t, d, min)
	require.LessOrEqual(t, d, max)
}

type fakeRuntime struct {
	connectStatus  mcpclient.RuntimeStatus
	status         mcpclient.RuntimeStatus
	tools          []mcp.Tool
	toolResult     *mcp.CallToolResult
	connectErr     error
	listToolsErr   error
	callToolErr    error
	callToolFn     func(context.Context, string, string, map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error)
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

func (f *fakeRuntime) CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	if f.callToolFn != nil {
		return f.callToolFn(ctx, serviceID, name, args)
	}
	return f.toolResult, f.status, f.callToolErr
}

func setupRuntimePortTest(t *testing.T) (repository.MCPServiceRepository, repository.ToolRepository, repository.RequestHistoryRepository) {
	t.Helper()

	db, err := database.Init(config.DatabaseConfig{
		Driver:       "sqlite",
		DSN:          ":memory:",
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	t.Cleanup(func() { _ = database.Close() })
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
			ServiceID:      "svc-fake",
			Status:         entity.ServiceStatusConnected,
			TransportType:  string(entity.TransportTypeStreamableHTTP),
			ExecutorID:     "executor@test@127.0.0.1:18081",
			SnapshotWriter: "executor@test@127.0.0.1:18081",
			RequestID:      "rpc-status-1",
			FailureCount:   0,
		},
	}

	svc := NewMCPService(serviceRepo, toolRepo, runtime, runtime, NoopAuditSink{}, nil)
	status, err := svc.Status(ctx, "svc-fake")
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusConnected, status["status"])
	require.Equal(t, entity.ServiceStatusDisconnected, status["persisted_status"])
	require.Equal(t, entity.ServiceStatusConnected, status["runtime_status"])
	require.Equal(t, "runtime", status["status_source"])
	require.Equal(t, "executor@test@127.0.0.1:18081", status["executor_id"])
	require.Equal(t, "executor@test@127.0.0.1:18081", status["snapshot_writer"])
	require.Equal(t, "rpc-status-1", status["request_id"])
}

func TestMCPServiceStatusIncludesIdleDiagnosticsFromRuntime(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, _ := setupRuntimePortTest(t)
	require.NoError(t, serviceRepo.Create(ctx, &entity.MCPService{
		Base:          entity.Base{ID: "svc-idle"},
		Name:          "svc-idle",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-idle.test/mcp",
		Status:        entity.ServiceStatusDisconnected,
	}))

	now := time.Now()
	lastUsedAt := now.Add(-2 * time.Minute)
	connectedAt := now.Add(-10 * time.Minute)
	runtime := &fakeRuntime{
		status: mcpclient.RuntimeStatus{
			ServiceID:       "svc-idle",
			Status:          entity.ServiceStatusConnected,
			TransportType:   string(entity.TransportTypeStreamableHTTP),
			ListenEnabled:   false,
			LastUsedAt:      &lastUsedAt,
			ConnectedAt:     &connectedAt,
			SessionIDExists: true,
		},
	}

	svc := NewMCPService(
		serviceRepo,
		toolRepo,
		runtime,
		runtime,
		NoopAuditSink{},
		nil,
		WithRuntimeConfig(config.RuntimeConfig{IdleTimeout: time.Minute}),
	)
	status, err := svc.Status(ctx, "svc-idle")
	require.NoError(t, err)
	requireDurationStringInRange(t, status["idle_duration"], 2*time.Minute, 2*time.Minute+time.Second)
	requireDurationStringInRange(t, status["connected_duration"], 10*time.Minute, 10*time.Minute+time.Second)
	require.Equal(t, true, status["would_reap"])
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

func TestMCPServiceStatusFallsBackToFreshSnapshotWhenRuntimeMissing(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, _ := setupRuntimePortTest(t)
	require.NoError(t, serviceRepo.Create(ctx, &entity.MCPService{
		Base:          entity.Base{ID: "svc-snapshot"},
		Name:          "svc-snapshot",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-snapshot.test/mcp",
		Status:        entity.ServiceStatusDisconnected,
	}))

	store := NewMemoryRuntimeStore()
	now := time.Now()
	require.NoError(t, store.SaveSnapshot(ctx, mcpclient.RuntimeSnapshot{
		RuntimeStatus: mcpclient.RuntimeStatus{
			ServiceID:      "svc-snapshot",
			Status:         entity.ServiceStatusConnected,
			TransportType:  string(entity.TransportTypeStreamableHTTP),
			ExecutorID:     "executor@test@127.0.0.1:18081",
			SnapshotWriter: "executor@test@127.0.0.1:18081",
			RequestID:      "rpc-snapshot-1",
			LastUsedAt:     &now,
			InFlight:       2,
		},
		ObservedAt: now,
	}))

	runtime := &fakeRuntime{}
	svc := NewMCPService(
		serviceRepo,
		toolRepo,
		runtime,
		runtime,
		NoopAuditSink{},
		nil,
		WithRuntimeSnapshotStore(store),
		WithRuntimeConfig(config.RuntimeConfig{SnapshotTTL: time.Minute}),
	)
	status, err := svc.Status(ctx, "svc-snapshot")
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusConnected, status["status"])
	require.Equal(t, "snapshot", status["status_source"])
	require.Equal(t, "fresh", status["snapshot_freshness"])
	require.Equal(t, 2, status["in_flight"])
	require.Equal(t, "executor@test@127.0.0.1:18081", status["executor_id"])
	require.Equal(t, "executor@test@127.0.0.1:18081", status["snapshot_writer"])
	require.Equal(t, "rpc-snapshot-1", status["request_id"])
}

func TestMCPServiceStatusIgnoresStaleSnapshot(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, _ := setupRuntimePortTest(t)
	require.NoError(t, serviceRepo.Create(ctx, &entity.MCPService{
		Base:          entity.Base{ID: "svc-stale"},
		Name:          "svc-stale",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-stale.test/mcp",
		Status:        entity.ServiceStatusDisconnected,
	}))

	store := NewMemoryRuntimeStore()
	require.NoError(t, store.SaveSnapshot(ctx, mcpclient.RuntimeSnapshot{
		RuntimeStatus: mcpclient.RuntimeStatus{
			ServiceID:     "svc-stale",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
		},
		ObservedAt: time.Now().Add(-2 * time.Minute),
	}))

	runtime := &fakeRuntime{}
	svc := NewMCPService(
		serviceRepo,
		toolRepo,
		runtime,
		runtime,
		NoopAuditSink{},
		nil,
		WithRuntimeSnapshotStore(store),
		WithRuntimeConfig(config.RuntimeConfig{SnapshotTTL: time.Second}),
	)
	status, err := svc.Status(ctx, "svc-stale")
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusDisconnected, status["status"])
	require.Equal(t, "persisted", status["status_source"])
	require.Equal(t, "stale", status["snapshot_freshness"])
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

func TestToolInvokeServiceInvokeRejectsWhenServiceRateLimited(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, historyRepo := setupRuntimePortTest(t)
	require.NoError(t, serviceRepo.Create(ctx, &entity.MCPService{Base: entity.Base{ID: "svc-rate"}, Name: "svc-rate", TransportType: entity.TransportTypeStreamableHTTP, URL: "http://svc-rate.test/mcp", Status: entity.ServiceStatusConnected}))
	require.NoError(t, toolRepo.Create(ctx, &entity.Tool{Base: entity.Base{ID: "tool-rate"}, MCPServiceID: "svc-rate", Name: "echo", IsEnabled: true}))

	runtime := &fakeRuntime{status: mcpclient.RuntimeStatus{ServiceID: "svc-rate", Status: entity.ServiceStatusConnected, TransportType: string(entity.TransportTypeStreamableHTTP)}, toolResult: &mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent("ok")}}}
	svc := NewToolInvokeService(
		config.HistoryConfig{MaxBodyBytes: 4096, Compression: "none"},
		toolRepo,
		serviceRepo,
		historyRepo,
		runtime,
		WithToolInvokeExecutionConfig(config.ExecutionConfig{ServiceRateLimit: 1, RateLimitWindow: time.Minute}),
	)

	_, err := svc.Invoke(ctx, "tool-rate", map[string]any{"text": "first"}, AuditEntry{UserID: "u-1", Username: "tester"})
	require.NoError(t, err)
	_, err = svc.Invoke(ctx, "tool-rate", map[string]any{"text": "second"}, AuditEntry{UserID: "u-1", Username: "tester"})
	require.Error(t, err)
	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, http.StatusTooManyRequests, bizErr.HTTPStatus)
	require.Equal(t, response.CodeTooManyRequests, bizErr.Code)
}

func TestToolInvokeServiceInvokeRejectsWhenExecutorConcurrencyExceeded(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, historyRepo := setupRuntimePortTest(t)
	require.NoError(t, serviceRepo.Create(ctx, &entity.MCPService{Base: entity.Base{ID: "svc-concurrency"}, Name: "svc-concurrency", TransportType: entity.TransportTypeStreamableHTTP, URL: "http://svc-concurrency.test/mcp", Status: entity.ServiceStatusConnected}))
	require.NoError(t, toolRepo.Create(ctx, &entity.Tool{Base: entity.Base{ID: "tool-concurrency"}, MCPServiceID: "svc-concurrency", Name: "echo", IsEnabled: true}))

	started := make(chan struct{})
	release := make(chan struct{})
	runtime := &fakeRuntime{status: mcpclient.RuntimeStatus{ServiceID: "svc-concurrency", Status: entity.ServiceStatusConnected, TransportType: string(entity.TransportTypeStreamableHTTP)}}
	runtime.callToolFn = func(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
			return &mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent("ok")}}, runtime.status, nil
		case <-ctx.Done():
			return nil, runtime.status, ctx.Err()
		}
	}
	svc := NewToolInvokeService(
		config.HistoryConfig{MaxBodyBytes: 4096, Compression: "none"},
		toolRepo,
		serviceRepo,
		historyRepo,
		runtime,
		WithToolInvokeExecutionConfig(config.ExecutionConfig{ExecutorConcurrency: 1, RateLimitWindow: time.Minute}),
	)

	errCh := make(chan error, 1)
	go func() {
		_, invokeErr := svc.Invoke(ctx, "tool-concurrency", map[string]any{"text": "first"}, AuditEntry{UserID: "u-1", Username: "tester"})
		errCh <- invokeErr
	}()
	<-started
	_, err := svc.Invoke(ctx, "tool-concurrency", map[string]any{"text": "second"}, AuditEntry{UserID: "u-2", Username: "tester-2"})
	require.Error(t, err)
	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, http.StatusTooManyRequests, bizErr.HTTPStatus)
	close(release)
	require.NoError(t, <-errCh)
}

func TestToolInvokeServiceInvokeAsyncCancelAndTimeout(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, historyRepo := setupRuntimePortTest(t)
	require.NoError(t, serviceRepo.Create(ctx, &entity.MCPService{Base: entity.Base{ID: "svc-async"}, Name: "svc-async", TransportType: entity.TransportTypeStreamableHTTP, URL: "http://svc-async.test/mcp", Status: entity.ServiceStatusConnected}))
	require.NoError(t, toolRepo.Create(ctx, &entity.Tool{Base: entity.Base{ID: "tool-async"}, MCPServiceID: "svc-async", Name: "echo", IsEnabled: true}))

	runtime := &fakeRuntime{status: mcpclient.RuntimeStatus{ServiceID: "svc-async", Status: entity.ServiceStatusConnected, TransportType: string(entity.TransportTypeStreamableHTTP)}}
	runtime.callToolFn = func(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
		delay, _ := args["delay_ms"].(int)
		if delay == 0 {
			delay = 100
		}
		select {
		case <-time.After(time.Duration(delay) * time.Millisecond):
			return &mcp.CallToolResult{Content: []mcp.Content{mcp.NewTextContent("done")}}, runtime.status, nil
		case <-ctx.Done():
			return nil, runtime.status, ctx.Err()
		}
	}
	svc := NewToolInvokeService(
		config.HistoryConfig{MaxBodyBytes: 4096, Compression: "none"},
		toolRepo,
		serviceRepo,
		historyRepo,
		runtime,
		WithToolInvokeExecutionConfig(config.ExecutionConfig{AsyncInvokeEnabled: true, AsyncTaskQueueSize: 8, AsyncTaskWorkers: 1, DefaultTaskTimeout: 50 * time.Millisecond, RateLimitWindow: time.Minute}),
	)
	defer func() { require.NoError(t, svc.Stop(context.Background())) }()

	cancelTask, err := svc.InvokeAsync(ctx, "tool-async", map[string]any{"delay_ms": 200}, 0, AuditEntry{UserID: "u-1", Username: "tester", Role: entity.RoleOperator})
	require.NoError(t, err)
	_, err = svc.CancelTask(ctx, cancelTask.ID, AuditEntry{UserID: "u-1", Username: "tester", Role: entity.RoleOperator})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		item, getErr := svc.GetTask(ctx, cancelTask.ID, AuditEntry{UserID: "u-1", Username: "tester", Role: entity.RoleOperator})
		return getErr == nil && item.Status == AsyncTaskStatusCancelled
	}, time.Second, 20*time.Millisecond)

	timeoutTask, err := svc.InvokeAsync(ctx, "tool-async", map[string]any{"delay_ms": 200}, 10*time.Millisecond, AuditEntry{UserID: "u-1", Username: "tester", Role: entity.RoleOperator})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		item, getErr := svc.GetTask(ctx, timeoutTask.ID, AuditEntry{UserID: "u-1", Username: "tester", Role: entity.RoleOperator})
		return getErr == nil && item.Status == AsyncTaskStatusTimedOut
	}, time.Second, 20*time.Millisecond)
}
