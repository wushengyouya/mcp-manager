package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/mikasa/mcp-manager/internal/bootstrap"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/rpc"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/stretchr/testify/require"
)

// TestApp 聚合测试所需的应用依赖。
type TestApp struct {
	Engine *gin.Engine
	Role   string
}

// DualRoleTestApp 聚合 control-plane 与 executor 测试依赖。
type DualRoleTestApp struct {
	ControlPlane *TestApp
	ExecutorRPC  *httptest.Server
}

// DefaultTestConfig 返回测试应用默认配置。
func DefaultTestConfig(t *testing.T) config.Config {
	t.Helper()

	return config.Config{
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            18080,
			ReadTimeout:     time.Second,
			WriteTimeout:    time.Second,
			ShutdownTimeout: time.Second,
		},
		Database: config.DatabaseConfig{
			Driver:          "sqlite",
			DSN:             filepath.Join(t.TempDir(), "test.db"),
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: time.Hour,
		},
		JWT: config.JWTConfig{
			Issuer:     "issuer",
			Secret:     "secret",
			AccessTTL:  time.Hour,
			RefreshTTL: 24 * time.Hour,
		},
		Redis: config.RedisConfig{
			Enabled:          false,
			Addr:             "127.0.0.1:6379",
			KeyPrefix:        "mcp-manager:test:",
			DialTimeout:      time.Second,
			ReadTimeout:      time.Second,
			WriteTimeout:     time.Second,
			OperationTimeout: time.Second,
		},
		RPC: config.RPCConfig{
			Enabled:        false,
			ListenAddr:     "127.0.0.1:18081",
			ExecutorTarget: "http://127.0.0.1:18081",
			AuthToken:      "test-rpc-token",
			DialTimeout:    time.Second,
			RequestTimeout: 2 * time.Second,
		},
		Execution: config.ExecutionConfig{
			ExecutorConcurrency: 0,
			ServiceRateLimit:    0,
			UserRateLimit:       0,
			RateLimitWindow:     time.Minute,
			AsyncInvokeEnabled:  false,
			AsyncTaskQueueSize:  32,
			AsyncTaskWorkers:    2,
			DefaultTaskTimeout:  5 * time.Second,
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
			AsyncEnabled:    false,
			QueueSize:       64,
		},
		App: config.AppConfig{
			Name:              "test",
			Version:           "1.0.0",
			Role:              "all",
			InitAdminUsername: "root",
			InitAdminPassword: "admin123456",
			InitAdminEmail:    "root@example.com",
		},
		Runtime: config.RuntimeConfig{
			StatusSource:     "runtime_first",
			StartupReconcile: true,
			SnapshotEnabled:  false,
			SnapshotTTL:      30 * time.Second,
			IdleTimeout:      0,
		},
		History: config.HistoryConfig{
			MaxBodyBytes: 4096,
			Compression:  "none",
			AsyncEnabled: false,
			QueueSize:    64,
		},
		Alert: config.AlertConfig{
			Enabled:       false,
			SubjectPrefix: "[TEST]",
			SilenceWindow: time.Minute,
			AsyncEnabled:  false,
			QueueSize:     32,
		},
		Log: config.LogConfig{Level: "error", Format: "console", Output: "stdout"},
	}
}

// CloneDatabaseConfig 为测试矩阵克隆数据库配置，避免共享 sqlite 文件。
func CloneDatabaseConfig(t *testing.T, dbCfg config.DatabaseConfig) config.DatabaseConfig {
	t.Helper()
	cloned := dbCfg
	if cloned.Driver == "sqlite" {
		cloned.DSN = filepath.Join(t.TempDir(), "test.db")
	}
	return cloned
}

// TestAppBuilder 定义测试应用装配器。
type TestAppBuilder struct {
	t                   *testing.T
	cfg                 config.Config
	runtimeFactory      bootstrap.RuntimeFactory
	runtimePortsFactory bootstrap.RuntimePortsFactory
	jwtFactory          bootstrap.JWTFactory
	auditSinkFactory    bootstrap.AuditSinkFactory
}

// SetupTestApp 初始化完整应用，用于 integration 和 e2e 测试。
func SetupTestApp(t *testing.T) *TestApp {
	return NewTestAppBuilder(t).Build()
}

// NewTestAppBuilder 创建测试应用装配器。
func NewTestAppBuilder(t *testing.T) *TestAppBuilder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	require.NoError(t, logger.Init(config.LogConfig{Level: "error", Format: "console", Output: "stdout"}))

	return &TestAppBuilder{
		t:   t,
		cfg: DefaultTestConfig(t),
	}
}

// WithDatabaseConfig 覆盖测试数据库配置。
func (b *TestAppBuilder) WithDatabaseConfig(cfg config.DatabaseConfig) *TestAppBuilder {
	b.cfg.Database = cfg
	return b
}

// WithConfig 覆盖测试应用配置。
func (b *TestAppBuilder) WithConfig(cfg config.Config) *TestAppBuilder {
	b.cfg = cfg
	return b
}

// WithAppRole 覆盖测试应用角色。
func (b *TestAppBuilder) WithAppRole(role string) *TestAppBuilder {
	b.cfg.App.Role = role
	return b
}

// WithRPCConfig 覆盖测试 RPC 配置。
func (b *TestAppBuilder) WithRPCConfig(cfg config.RPCConfig) *TestAppBuilder {
	b.cfg.RPC = cfg
	return b
}

// WithRuntimeFactory 覆盖运行时管理器构造逻辑。
func (b *TestAppBuilder) WithRuntimeFactory(factory bootstrap.RuntimeFactory) *TestAppBuilder {
	b.runtimeFactory = factory
	return b
}

// WithRuntimePortsFactory 覆盖运行时端口构造逻辑。
func (b *TestAppBuilder) WithRuntimePortsFactory(factory bootstrap.RuntimePortsFactory) *TestAppBuilder {
	b.runtimePortsFactory = factory
	return b
}

// WithJWTFactory 覆盖 JWT 服务构造逻辑。
func (b *TestAppBuilder) WithJWTFactory(factory bootstrap.JWTFactory) *TestAppBuilder {
	b.jwtFactory = factory
	return b
}

// WithAuditSinkFactory 覆盖审计 sink 构造逻辑。
func (b *TestAppBuilder) WithAuditSinkFactory(factory bootstrap.AuditSinkFactory) *TestAppBuilder {
	b.auditSinkFactory = factory
	return b
}

// Build 构造测试应用。
func (b *TestAppBuilder) Build() *TestApp {
	b.t.Helper()

	builder := bootstrap.NewBuilder(b.cfg)
	if b.runtimeFactory != nil {
		builder = builder.WithRuntimeFactory(b.runtimeFactory)
	}
	if b.runtimePortsFactory != nil {
		builder = builder.WithRuntimePortsFactory(b.runtimePortsFactory)
	}
	if b.jwtFactory != nil {
		builder = builder.WithJWTFactory(b.jwtFactory)
	}
	if b.auditSinkFactory != nil {
		builder = builder.WithAuditSinkFactory(b.auditSinkFactory)
	}

	app, err := builder.Build()
	require.NoError(b.t, err)
	b.t.Cleanup(app.Cleanup)

	return &TestApp{Engine: app.Engine, Role: app.Role}
}

// DualRoleTestAppBuilder 构造 dual-role 测试依赖。
type DualRoleTestAppBuilder struct {
	t                      *testing.T
	cfg                    config.Config
	executorRuntimeFactory bootstrap.RuntimeFactory
	remoteStatusTimeout    time.Duration
}

// SetupDualRoleTestApp 初始化 control-plane + executor 双角色测试应用。
func SetupDualRoleTestApp(t *testing.T) *DualRoleTestApp {
	return NewDualRoleTestAppBuilder(t).Build()
}

// NewDualRoleTestAppBuilder 创建双角色测试应用装配器。
func NewDualRoleTestAppBuilder(t *testing.T) *DualRoleTestAppBuilder {
	t.Helper()

	cfg := DefaultTestConfig(t)
	cfg.App.Role = "control-plane"
	mini := miniredis.RunT(t)
	cfg.Redis.Enabled = true
	cfg.Redis.Addr = mini.Addr()
	cfg.Runtime.SnapshotEnabled = true
	cfg.Runtime.SnapshotTTL = time.Minute
	cfg.RPC.Enabled = true
	cfg.RPC.AuthToken = "integration-rpc-token"
	cfg.RPC.DialTimeout = time.Second
	cfg.RPC.RequestTimeout = 2 * time.Second

	return &DualRoleTestAppBuilder{
		t:                   t,
		cfg:                 cfg,
		remoteStatusTimeout: time.Second,
	}
}

// WithDatabaseConfig 覆盖 dual-role 测试数据库配置。
func (b *DualRoleTestAppBuilder) WithDatabaseConfig(cfg config.DatabaseConfig) *DualRoleTestAppBuilder {
	b.cfg.Database = cfg
	return b
}

// WithConfig 覆盖 dual-role control-plane 配置。
func (b *DualRoleTestAppBuilder) WithConfig(cfg config.Config) *DualRoleTestAppBuilder {
	b.cfg = cfg
	return b
}

// WithExecutorRuntimeFactory 覆盖 executor 运行时管理器构造逻辑。
func (b *DualRoleTestAppBuilder) WithExecutorRuntimeFactory(factory bootstrap.RuntimeFactory) *DualRoleTestAppBuilder {
	b.executorRuntimeFactory = factory
	return b
}

// WithRemoteStatusTimeout 覆盖 control-plane 远程状态查询超时。
func (b *DualRoleTestAppBuilder) WithRemoteStatusTimeout(timeout time.Duration) *DualRoleTestAppBuilder {
	if timeout > 0 {
		b.remoteStatusTimeout = timeout
	}
	return b
}

// Build 构造双角色测试应用。
func (b *DualRoleTestAppBuilder) Build() *DualRoleTestApp {
	b.t.Helper()

	cfg := b.cfg
	cfg.Database = CloneDatabaseConfig(b.t, cfg.Database)
	cfg.App.Role = "control-plane"

	executorFactory := b.executorRuntimeFactory
	if executorFactory == nil {
		executorFactory = func(appCfg config.AppConfig) *mcpclient.Manager {
			return mcpclient.NewManager(appCfg)
		}
	}

	executorStore := service.NewMemoryRuntimeStore()
	executorManager := executorFactory(config.AppConfig{
		Name:    cfg.App.Name,
		Version: cfg.App.Version,
		Role:    "executor",
	})
	executorAdapter := service.NewLocalRuntimeAdapter(executorManager, executorStore)
	executorServer := httptest.NewServer(rpc.NewHandler(executorHarness{adapter: executorAdapter}))
	b.t.Cleanup(executorServer.Close)

	cfg.RPC.ExecutorTarget = executorServer.URL

	controlPlane := NewTestAppBuilder(b.t).
		WithConfig(cfg).
		WithRuntimePortsFactory(func(cfg config.Config, manager *mcpclient.Manager, runtimeStore service.RuntimeStore) bootstrap.RuntimePorts {
			client := rpc.NewClient(cfg.RPC.ExecutorTarget, rpc.WithHTTPClient(executorServer.Client()))
			adapter := service.NewRemoteRuntimeAdapter(
				client,
				service.WithRemoteRuntimeSnapshotStore(runtimeStore),
				service.WithRemoteRuntimeStatusTimeout(b.remoteStatusTimeout),
			)
			return bootstrap.RuntimePorts{
				Connector:    adapter,
				StatusReader: adapter,
				ToolCatalog:  adapter,
				ToolInvoker:  adapter,
			}
		}).
		Build()

	return &DualRoleTestApp{
		ControlPlane: controlPlane,
		ExecutorRPC:  executorServer,
	}
}

type executorHarness struct {
	adapter *service.LocalRuntimeAdapter
}

func (h executorHarness) Connect(ctx context.Context, serviceItem *entity.MCPService) (mcpclient.RuntimeStatus, error) {
	return h.adapter.Connect(ctx, serviceItem)
}

func (h executorHarness) Disconnect(ctx context.Context, serviceID string) error {
	return h.adapter.Disconnect(ctx, serviceID)
}

func (h executorHarness) GetStatus(_ context.Context, serviceID string) (mcpclient.RuntimeStatus, bool, error) {
	status, ok := h.adapter.GetStatus(serviceID)
	return status, ok, nil
}

func (h executorHarness) ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error) {
	return h.adapter.ListTools(ctx, serviceID)
}

func (h executorHarness) CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	return h.adapter.CallTool(ctx, serviceID, name, args)
}

func (h executorHarness) Ping(context.Context) error {
	return nil
}

// BuildMCPServer 构建测试用的临时 MCP 服务。
func BuildMCPServer() *httptest.Server {
	srv := mcpserver.NewMCPServer("test-mcp", "1.0.0", mcpserver.WithToolCapabilities(true))
	srv.AddTool(
		mcp.NewTool(
			"echo",
			mcp.WithString("text", mcp.Required()),
			mcp.WithNumber("delay_ms"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args, _ := req.Params.Arguments.(map[string]any)
			text, _ := args["text"].(string)
			delay := intArg(args["delay_ms"])
			if delay > 0 {
				select {
				case <-time.After(time.Duration(delay) * time.Millisecond):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return mcp.NewToolResultText("echo:" + text), nil
		},
	)
	testSrv := httptest.NewUnstartedServer(mcpserver.NewStreamableHTTPServer(srv, mcpserver.WithStateful(true)))
	testSrv.Start()
	return testSrv
}

// NewHTTPServer 使用完整 Gin 引擎启动测试 HTTP 服务。
func intArg(v any) int {
	switch value := v.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func NewHTTPServer(t *testing.T, engine http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(engine)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}
