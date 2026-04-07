package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/mikasa/mcp-manager/internal/bootstrap"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/stretchr/testify/require"
)

// TestApp 聚合测试所需的应用依赖。
type TestApp struct {
	Engine *gin.Engine
}

// TestAppBuilder 定义测试应用装配器。
type TestAppBuilder struct {
	t                *testing.T
	cfg              config.Config
	runtimeFactory   bootstrap.RuntimeFactory
	jwtFactory       bootstrap.JWTFactory
	auditSinkFactory bootstrap.AuditSinkFactory
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
		t: t,
		cfg: config.Config{
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
			App: config.AppConfig{
				Name:              "test",
				Version:           "1.0.0",
				Role:              "all",
				InitAdminUsername: "root",
				InitAdminPassword: "admin123456",
				InitAdminEmail:    "root@example.com",
			},
			History: config.HistoryConfig{
				MaxBodyBytes: 4096,
				Compression:  "none",
			},
			Log: config.LogConfig{Level: "error", Format: "console", Output: "stdout"},
		},
	}
}

// WithConfig 覆盖测试应用配置。
func (b *TestAppBuilder) WithConfig(cfg config.Config) *TestAppBuilder {
	b.cfg = cfg
	return b
}

// WithRuntimeFactory 覆盖运行时管理器构造逻辑。
func (b *TestAppBuilder) WithRuntimeFactory(factory bootstrap.RuntimeFactory) *TestAppBuilder {
	b.runtimeFactory = factory
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
	if b.jwtFactory != nil {
		builder = builder.WithJWTFactory(b.jwtFactory)
	}
	if b.auditSinkFactory != nil {
		builder = builder.WithAuditSinkFactory(b.auditSinkFactory)
	}

	app, err := builder.Build()
	require.NoError(b.t, err)
	b.t.Cleanup(app.Cleanup)

	return &TestApp{Engine: app.Engine}
}

// BuildMCPServer 构建测试用的临时 MCP 服务。
func BuildMCPServer() *httptest.Server {
	srv := mcpserver.NewMCPServer("test-mcp", "1.0.0", mcpserver.WithToolCapabilities(true))
	srv.AddTool(mcp.NewTool("echo", mcp.WithString("text", mcp.Required())), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		text, _ := args["text"].(string)
		return mcp.NewToolResultText("echo:" + text), nil
	})
	testSrv := httptest.NewUnstartedServer(mcpserver.NewStreamableHTTPServer(srv, mcpserver.WithStateful(true)))
	testSrv.Start()
	return testSrv
}

// NewHTTPServer 使用完整 Gin 引擎启动测试 HTTP 服务。
func NewHTTPServer(t *testing.T, engine http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(engine)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}
