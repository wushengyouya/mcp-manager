package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/bootstrap"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/database"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/stretchr/testify/require"
)

type disconnecterStub struct {
	calls []string
}

func (d *disconnecterStub) Disconnect(_ context.Context, serviceID string) error {
	d.calls = append(d.calls, serviceID)
	return nil
}

type auditSinkStub struct {
	entries []service.AuditEntry
}

func (a *auditSinkStub) Record(_ context.Context, entry service.AuditEntry) error {
	a.entries = append(a.entries, entry)
	return nil
}

type alertServiceStub struct {
	calls []struct {
		serviceName   string
		transportType string
		endpoint      string
		reason        string
	}
}

func (a *alertServiceStub) NotifyServiceError(_ context.Context, serviceName, transportType, endpoint, reason string) error {
	a.calls = append(a.calls, struct {
		serviceName   string
		transportType string
		endpoint      string
		reason        string
	}{serviceName: serviceName, transportType: transportType, endpoint: endpoint, reason: reason})
	return nil
}

func setupServerRepoTest(t *testing.T) repository.MCPServiceRepository {
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
	return repository.NewMCPServiceRepository(db)
}

func TestNewHealthUpdateFnUpdatesStatusWithoutAuditOnHealthyResult(t *testing.T) {
	repo := setupServerRepoTest(t)
	item := &entity.MCPService{
		Name:          "svc-ok",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-ok.test/mcp",
		Status:        entity.ServiceStatusConnected,
	}
	require.NoError(t, repo.Create(context.Background(), item))

	disconnecter := &disconnecterStub{}
	auditSink := &auditSinkStub{}
	alertSvc := &alertServiceStub{}
	updateFn := newHealthUpdateFn(repo, disconnecter, auditSink, alertSvc)

	require.NoError(t, updateFn(context.Background(), item.ID, entity.ServiceStatusConnected, 1, ""))
	updated, err := repo.GetByID(context.Background(), item.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusConnected, updated.Status)
	require.Equal(t, 1, updated.FailureCount)
	require.Empty(t, disconnecter.calls)
	require.Empty(t, auditSink.entries)
	require.Empty(t, alertSvc.calls)
}

func TestNewHealthUpdateFnFirstErrorAuditsAndAlerts(t *testing.T) {
	repo := setupServerRepoTest(t)
	item := &entity.MCPService{
		Name:          "svc-error",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-error.test/mcp",
		Status:        entity.ServiceStatusConnected,
		ListenEnabled: true,
	}
	require.NoError(t, repo.Create(context.Background(), item))

	disconnecter := &disconnecterStub{}
	auditSink := &auditSinkStub{}
	alertSvc := &alertServiceStub{}
	updateFn := newHealthUpdateFn(repo, disconnecter, auditSink, alertSvc)

	require.NoError(t, updateFn(context.Background(), item.ID, entity.ServiceStatusError, 2, "boom"))
	updated, err := repo.GetByID(context.Background(), item.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusError, updated.Status)
	require.Equal(t, 2, updated.FailureCount)
	require.Equal(t, []string{item.ID}, disconnecter.calls)
	require.Len(t, auditSink.entries, 1)
	require.Equal(t, "service_error", auditSink.entries[0].Action)
	require.Len(t, alertSvc.calls, 1)
	require.Equal(t, "svc-error", alertSvc.calls[0].serviceName)
	require.Equal(t, "http://svc-error.test/mcp", alertSvc.calls[0].endpoint)
}

func TestNewHealthUpdateFnRepeatedErrorDoesNotDuplicateSideEffects(t *testing.T) {
	repo := setupServerRepoTest(t)
	item := &entity.MCPService{
		Name:          "svc-repeat-error",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-repeat.test/mcp",
		Status:        entity.ServiceStatusError,
	}
	require.NoError(t, repo.Create(context.Background(), item))

	disconnecter := &disconnecterStub{}
	auditSink := &auditSinkStub{}
	alertSvc := &alertServiceStub{}
	updateFn := newHealthUpdateFn(repo, disconnecter, auditSink, alertSvc)

	require.NoError(t, updateFn(context.Background(), item.ID, entity.ServiceStatusError, 3, "still broken"))
	require.Equal(t, []string{item.ID}, disconnecter.calls)
	require.Empty(t, auditSink.entries)
	require.Empty(t, alertSvc.calls)
}

func TestNewHealthUpdateFnReturnsLookupError(t *testing.T) {
	repo := setupServerRepoTest(t)
	updateFn := newHealthUpdateFn(repo, &disconnecterStub{}, &auditSinkStub{}, &alertServiceStub{})
	err := updateFn(context.Background(), "missing", entity.ServiceStatusError, 1, "boom")
	require.ErrorIs(t, err, repository.ErrNotFound)
}

func TestBuildAppConstructsManagedServerAndHandler(t *testing.T) {
	require.NoError(t, logger.Init(config.LogConfig{Level: "info", Format: "console", Output: "stdout"}))

	cfg := config.Config{
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            18080,
			ReadTimeout:     time.Second,
			WriteTimeout:    2 * time.Second,
			ShutdownTimeout: time.Second,
		},
		Database: config.DatabaseConfig{
			Driver:       "sqlite",
			DSN:          filepath.Join(t.TempDir(), "mcp-manager.db"),
			MaxOpenConns: 1,
			MaxIdleConns: 1,
		},
		JWT: config.JWTConfig{
			Issuer:     "test",
			Secret:     "test-secret",
			AccessTTL:  time.Hour,
			RefreshTTL: 24 * time.Hour,
		},
		HealthCheck: config.HealthCheckConfig{
			Enabled:          false,
			Interval:         time.Second,
			Timeout:          time.Second,
			FailureThreshold: 1,
		},
		Audit: config.AuditConfig{
			RetentionDays:   7,
			CleanupInterval: time.Hour,
		},
		Alert: config.AlertConfig{Enabled: false},
		Log:   config.LogConfig{Level: "info", Format: "console", Output: "stdout"},
		App: config.AppConfig{
			Name:              "mcp-manager",
			Version:           "test",
			InitAdminUsername: "root",
			InitAdminPassword: "admin123456",
			InitAdminEmail:    "root@example.com",
		},
		History: config.HistoryConfig{
			MaxBodyBytes: 8192,
			Compression:  "none",
		},
	}

	app, err := buildApp(cfg)
	require.NoError(t, err)
	require.NotNil(t, app)
	require.NotNil(t, app.cleanup)
	require.Len(t, app.servers, 1)
	require.Equal(t, "http", app.servers[0].name)
	require.Equal(t, "127.0.0.1:18080", app.servers[0].server.Addr)
	require.Equal(t, time.Second, app.servers[0].server.ReadTimeout)
	require.Equal(t, 2*time.Second, app.servers[0].server.WriteTimeout)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	app.servers[0].server.Handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	app.cleanup()
}

func TestCollectBootstrapServersIncludesPrimaryServer(t *testing.T) {
	primary := &http.Server{Addr: "127.0.0.1:18080"}
	servers := collectBootstrapServers(&bootstrap.App{Server: primary})
	require.Len(t, servers, 1)
	require.Equal(t, "http", servers[0].name)
	require.Same(t, primary, servers[0].server)
}

func TestAppendReflectedServersSupportsPointerAndSlice(t *testing.T) {
	primary := &http.Server{Addr: "127.0.0.1:18080"}
	rpc := &http.Server{Addr: "127.0.0.1:19090"}
	seen := map[*http.Server]struct{}{}
	servers := make([]namedHTTPServer, 0, 2)
	appendServer := func(name string, srv *http.Server) {
		if _, ok := seen[srv]; ok {
			return
		}
		seen[srv] = struct{}{}
		servers = append(servers, namedHTTPServer{name: name, server: srv})
	}
	serverType := reflect.TypeOf(&http.Server{})

	appendReflectedServers("RPCServer", rpc, serverType, appendServer)
	appendReflectedServers("Servers", []*http.Server{primary, rpc}, serverType, appendServer)

	require.Len(t, servers, 2)
	require.Equal(t, "RPCServer", servers[0].name)
	require.Same(t, rpc, servers[0].server)
	require.Equal(t, "Servers_1", servers[1].name)
	require.Same(t, primary, servers[1].server)
}

func TestItoaZapErrorAndEndpointOf(t *testing.T) {
	require.Equal(t, "8080", itoa(8080))
	require.Equal(t, "error", zapError(errors.New("boom")).Key)
	require.Equal(t, "", endpointOf(nil))
	require.Equal(t, "http://svc.test/mcp", endpointOf(&entity.MCPService{URL: "http://svc.test/mcp"}))
	require.Equal(t, "echo", endpointOf(&entity.MCPService{Command: "echo"}))
}
