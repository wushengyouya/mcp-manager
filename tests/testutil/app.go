package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/database"
	"github.com/mikasa/mcp-manager/internal/handler"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/router"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/mikasa/mcp-manager/scripts"
	"github.com/stretchr/testify/require"
)

// TestApp 聚合测试所需的应用依赖。
type TestApp struct {
	Engine *gin.Engine
}

// SetupTestApp 初始化完整应用，用于 integration 和 e2e 测试。
func SetupTestApp(t *testing.T) *TestApp {
	t.Helper()

	gin.SetMode(gin.TestMode)
	require.NoError(t, logger.Init(config.LogConfig{Level: "error", Format: "console", Output: "stdout"}))

	db, err := database.Init(config.DatabaseConfig{
		Driver:          "sqlite",
		DSN:             ":memory:",
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Hour,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, database.Migrate(db))

	userRepo := repository.NewUserRepository(db)
	serviceRepo := repository.NewMCPServiceRepository(db)
	toolRepo := repository.NewToolRepository(db)
	historyRepo := repository.NewRequestHistoryRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)
	require.NoError(t, scripts.EnsureAdmin(context.Background(), userRepo, "root", "admin123456", "root@example.com"))

	jwtSvc := crypto.NewJWTService("secret", "issuer", time.Hour, 24*time.Hour, crypto.NewTokenBlacklist())
	auditSink := service.NewDBAuditSink(auditRepo)
	authSvc := service.NewAuthService(userRepo, jwtSvc, auditSink)
	userSvc := service.NewUserService(userRepo, auditSink)
	manager := mcpclient.NewManager(config.AppConfig{Name: "test", Version: "1.0.0"})
	mcpSvc := service.NewMCPService(serviceRepo, toolRepo, manager, auditSink, nil)
	toolSvc := service.NewToolService(toolRepo, serviceRepo, manager, auditSink)
	invokeSvc := service.NewToolInvokeService(
		config.HistoryConfig{MaxBodyBytes: 4096, Compression: "none"},
		toolRepo,
		serviceRepo,
		historyRepo,
		manager,
	)
	auditSvc := service.NewAuditService(auditSink, auditRepo)

	engine := router.New(jwtSvc, router.Handlers{
		Auth:    handler.NewAuthHandler(authSvc),
		User:    handler.NewUserHandler(userSvc, authSvc),
		MCP:     handler.NewMCPHandler(mcpSvc),
		Tool:    handler.NewToolHandler(toolSvc, invokeSvc),
		History: handler.NewHistoryHandler(historyRepo),
		Audit:   handler.NewAuditHandler(auditSvc),
	})

	return &TestApp{Engine: engine}
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
