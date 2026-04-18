package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/database"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/handler"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/router"
	"github.com/mikasa/mcp-manager/internal/rpc"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/internal/task"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/email"
	"github.com/mikasa/mcp-manager/scripts"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type serviceDisconnecter interface {
	Disconnect(ctx context.Context, serviceID string) error
}

// RuntimeFactory 定义运行时管理器构造函数。
type RuntimeFactory func(appCfg config.AppConfig) *mcpclient.Manager

// JWTFactory 定义 JWT 服务构造函数。
type JWTFactory func(cfg config.Config, blacklistStore appcrypto.TokenBlacklistStore) *appcrypto.JWTService

// AuditSinkFactory 定义审计 sink 构造函数。
type AuditSinkFactory func(repo repository.AuditLogRepository) service.AuditSink

// HealthChecker 定义健康检查器最小契约。
type HealthChecker interface {
	Start()
	Stop()
}

// HealthCheckerFactory 定义健康检查器构造函数。
type HealthCheckerFactory func(manager *mcpclient.Manager, cfg config.HealthCheckConfig, updateFn func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error) HealthChecker

// CleanupTask 定义后台清理任务最小契约。
type CleanupTask interface {
	Start()
	Stop()
}

// AuditCleanupTaskFactory 定义审计清理任务构造函数。
type AuditCleanupTaskFactory func(repo repository.AuditLogRepository, retentionDays int, interval time.Duration) CleanupTask

// RuntimePorts 聚合运行时相关端口，便于按角色切换本地或远程实现。
type RuntimePorts struct {
	Connector    service.ServiceConnector
	StatusReader service.RuntimeStatusReader
	ToolCatalog  service.ToolCatalogExecutor
	ToolInvoker  service.ToolInvoker
}

// RuntimePortsFactory 定义运行时端口构造函数。
type RuntimePortsFactory func(cfg config.Config, manager *mcpclient.Manager, runtimeStore service.RuntimeStore) RuntimePorts

type appRole string

const (
	appRoleAll          appRole = "all"
	appRoleControlPlane appRole = "control-plane"
	appRoleExecutor     appRole = "executor"
)

func resolveAppRole(raw string) appRole {
	if raw == "" {
		return appRoleAll
	}
	return appRole(raw)
}

func (r appRole) runsControlPlane() bool {
	return r != appRoleExecutor
}

func (r appRole) runsLocalRuntime() bool {
	return r != appRoleControlPlane
}

// App 保存应用装配产物，供主程序与测试复用。
type App struct {
	Engine    *gin.Engine
	Server    *http.Server
	RPCServer *http.Server
	Cleanup   func()
	Role      string
}

// Builder 定义应用装配器。
type Builder struct {
	cfg                     config.Config
	runtimeFactory          RuntimeFactory
	runtimePortsFactory     RuntimePortsFactory
	jwtFactory              JWTFactory
	auditSinkFactory        AuditSinkFactory
	healthCheckerFactory    HealthCheckerFactory
	auditCleanupTaskFactory AuditCleanupTaskFactory
}

// NewBuilder 创建应用装配器。
func NewBuilder(cfg config.Config) *Builder {
	return &Builder{
		cfg:                  cfg,
		runtimeFactory:       func(appCfg config.AppConfig) *mcpclient.Manager { return mcpclient.NewManager(appCfg) },
		runtimePortsFactory:  defaultRuntimePortsFactory,
		jwtFactory:           defaultJWTFactory,
		auditSinkFactory:     func(repo repository.AuditLogRepository) service.AuditSink { return service.NewDBAuditSink(repo) },
		healthCheckerFactory: defaultHealthCheckerFactory,
		auditCleanupTaskFactory: func(repo repository.AuditLogRepository, retentionDays int, interval time.Duration) CleanupTask {
			return task.NewAuditCleanupTask(repo, retentionDays, interval)
		},
	}
}

// WithRuntimeFactory 覆盖运行时管理器构造逻辑。
func (b *Builder) WithRuntimeFactory(factory RuntimeFactory) *Builder {
	if factory != nil {
		b.runtimeFactory = factory
	}
	return b
}

// WithRuntimePortsFactory 覆盖运行时端口构造逻辑。
func (b *Builder) WithRuntimePortsFactory(factory RuntimePortsFactory) *Builder {
	if factory != nil {
		b.runtimePortsFactory = factory
	}
	return b
}

// WithJWTFactory 覆盖 JWT 服务构造逻辑。
func (b *Builder) WithJWTFactory(factory JWTFactory) *Builder {
	if factory != nil {
		b.jwtFactory = factory
	}
	return b
}

// WithAuditSinkFactory 覆盖审计 sink 构造逻辑。
func (b *Builder) WithAuditSinkFactory(factory AuditSinkFactory) *Builder {
	if factory != nil {
		b.auditSinkFactory = factory
	}
	return b
}

// WithHealthCheckerFactory 覆盖健康检查器构造逻辑。
func (b *Builder) WithHealthCheckerFactory(factory HealthCheckerFactory) *Builder {
	if factory != nil {
		b.healthCheckerFactory = factory
	}
	return b
}

// WithAuditCleanupTaskFactory 覆盖审计清理任务构造逻辑。
func (b *Builder) WithAuditCleanupTaskFactory(factory AuditCleanupTaskFactory) *Builder {
	if factory != nil {
		b.auditCleanupTaskFactory = factory
	}
	return b
}

// Build 执行应用装配。
func (b *Builder) Build() (*App, error) {
	role := resolveAppRole(b.cfg.App.Role)
	db, err := database.Init(b.cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("初始化数据库失败: %w", err)
	}

	cleanupFns := []func(){func() { _ = database.Close() }}
	cleanup := func() {
		for i := len(cleanupFns) - 1; i >= 0; i-- {
			cleanupFns[i]()
		}
	}

	if role.runsControlPlane() {
		if err := database.Migrate(db); err != nil {
			cleanup()
			return nil, fmt.Errorf("执行迁移失败: %w", err)
		}
	}

	userRepo := repository.NewUserRepository(db)
	authSessionRepo := repository.NewAuthSessionRepository(db)
	serviceRepo := repository.NewMCPServiceRepository(db)
	toolRepo := repository.NewToolRepository(db)
	historyRepo := repository.NewRequestHistoryRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	if role.runsControlPlane() {
		if err := scripts.EnsureAdmin(context.Background(), userRepo, b.cfg.App.InitAdminUsername, b.cfg.App.InitAdminPassword, b.cfg.App.InitAdminEmail); err != nil {
			cleanup()
			return nil, fmt.Errorf("初始化管理员失败: %w", err)
		}
	}
	if role.runsLocalRuntime() && shouldStartupReconcile(b.cfg.Runtime) {
		if role.runsControlPlane() || runtimeSchemaReady(db) {
			if err := reconcileStartupStatuses(context.Background(), serviceRepo); err != nil {
				cleanup()
				return nil, fmt.Errorf("修正启动状态失败: %w", err)
			}
		}
	}

	var redisClient redis.UniversalClient
	if b.cfg.Redis.Enabled {
		redisClient = newRedisClient(b.cfg.Redis)
		cleanupFns = append(cleanupFns, func() { _ = redisClient.Close() })
	}

	blacklistStore := buildTokenBlacklistStore(redisClient, b.cfg)
	userTokenVersionStore := buildUserTokenVersionStore(redisClient, b.cfg)
	sessionStateStore := buildSessionStateStore(redisClient, b.cfg)
	runtimeStore := buildRuntimeStore(redisClient, b.cfg)
	jwtSvc := b.jwtFactory(b.cfg, blacklistStore)
	authStateManager := service.NewAuthStateManager(userRepo, authSessionRepo, userTokenVersionStore, sessionStateStore)
	jwtSvc.SetAccessTokenValidator(authStateManager)
	auditSink := b.auditSinkFactory(auditRepo)
	if b.cfg.Audit.AsyncEnabled {
		asyncAuditSink := service.NewAsyncAuditSink(auditSink, 1, b.cfg.Audit.QueueSize)
		cleanupFns = append(cleanupFns, func() { _ = asyncAuditSink.Stop(context.Background()) })
		auditSink = asyncAuditSink
	}
	authSvc := service.NewAuthService(userRepo, authSessionRepo, jwtSvc, auditSink, service.WithAuthStateManager(authStateManager))
	userSvc := service.NewUserService(userRepo, auditSink, service.WithUserAuthStateManager(authStateManager))
	var manager *mcpclient.Manager
	if role.runsLocalRuntime() {
		manager = b.runtimeFactory(b.cfg.App)
	}
	runtimePorts := b.runtimePortsFactory(b.cfg, manager, runtimeStore)
	auditSvc := service.NewAuditService(auditSink, auditRepo)

	var sender email.Sender
	if b.cfg.Alert.Enabled {
		sender = email.NewSMTPSender(b.cfg.Alert.SMTPHost, b.cfg.Alert.SMTPPort, b.cfg.Alert.SMTPUsername, b.cfg.Alert.SMTPPassword)
	}
	alertSvc := service.NewAlertService(b.cfg.Alert, sender)
	if b.cfg.Alert.AsyncEnabled {
		asyncAlertSvc := service.NewAsyncAlertService(alertSvc, 1, b.cfg.Alert.QueueSize)
		cleanupFns = append(cleanupFns, func() { _ = asyncAlertSvc.Stop(context.Background()) })
		alertSvc = asyncAlertSvc
	}
	historySink := service.NewDBHistorySink(historyRepo)
	if b.cfg.History.AsyncEnabled {
		workers := b.cfg.Execution.AsyncTaskWorkers
		if workers <= 0 {
			workers = 1
		}
		asyncHistorySink := service.NewAsyncHistorySink(historySink, workers, b.cfg.History.QueueSize)
		cleanupFns = append(cleanupFns, func() { _ = asyncHistorySink.Stop(context.Background()) })
		historySink = asyncHistorySink
	}
	invokeController := service.NewInvokeController(b.cfg.Execution)
	mcpSvc := service.NewMCPService(
		serviceRepo,
		toolRepo,
		runtimePorts.Connector,
		runtimePorts.StatusReader,
		auditSink,
		alertSvc,
		service.WithRuntimeSnapshotStore(runtimeStore),
		service.WithRuntimeConfig(b.cfg.Runtime),
	)
	toolSvc := service.NewToolService(toolRepo, serviceRepo, runtimePorts.ToolCatalog, auditSink)
	invokeSvc := service.NewToolInvokeService(
		b.cfg.History,
		toolRepo,
		serviceRepo,
		historyRepo,
		runtimePorts.ToolInvoker,
		service.WithToolInvokeExecutionConfig(b.cfg.Execution),
		service.WithToolInvokeHistorySink(historySink),
		service.WithToolInvokeController(invokeController),
	)
	cleanupFns = append(cleanupFns, func() { _ = invokeSvc.Stop(context.Background()) })

	if role.runsLocalRuntime() && b.cfg.HealthCheck.Enabled {
		healthChecker := b.healthCheckerFactory(manager, b.cfg.HealthCheck, newHealthUpdateFn(serviceRepo, manager, auditSink, alertSvc))
		healthChecker.Start()
		cleanupFns = append(cleanupFns, healthChecker.Stop)
	}

	if role.runsControlPlane() {
		cleanupTask := b.auditCleanupTaskFactory(auditRepo, b.cfg.Audit.RetentionDays, b.cfg.Audit.CleanupInterval)
		cleanupTask.Start()
		cleanupFns = append(cleanupFns, cleanupTask.Stop)
	}

	engine := router.New(jwtSvc, router.Handlers{
		Auth:    handler.NewAuthHandler(authSvc),
		User:    handler.NewUserHandler(userSvc, authSvc),
		MCP:     handler.NewMCPHandler(mcpSvc),
		Tool:    handler.NewToolHandler(toolSvc, invokeSvc),
		History: handler.NewHistoryHandler(historyRepo),
		Audit:   handler.NewAuditHandler(auditSvc),
	},
		router.WithRole(b.cfg.App.Role),
		router.WithReadinessProbe(buildReadinessProbe(role, db, b.cfg, runtimePorts)),
	)

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", b.cfg.Server.Host, b.cfg.Server.Port),
		Handler:      engine,
		ReadTimeout:  b.cfg.Server.ReadTimeout,
		WriteTimeout: b.cfg.Server.WriteTimeout,
	}

	var rpcServer *http.Server
	if role == appRoleExecutor && b.cfg.RPC.Enabled {
		rpcServer = &http.Server{
			Addr:         b.cfg.RPC.ListenAddr,
			Handler:      rpc.NewHandler(newRPCExecutor(runtimePorts)),
			ReadTimeout:  b.cfg.Server.ReadTimeout,
			WriteTimeout: b.cfg.Server.WriteTimeout,
		}
	}

	return &App{
		Engine:    engine,
		Server:    srv,
		RPCServer: rpcServer,
		Cleanup:   cleanup,
		Role:      b.cfg.App.Role,
	}, nil
}

func defaultJWTFactory(cfg config.Config, blacklistStore appcrypto.TokenBlacklistStore) *appcrypto.JWTService {
	return appcrypto.NewJWTService(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL, blacklistStore)
}

func defaultHealthCheckerFactory(manager *mcpclient.Manager, cfg config.HealthCheckConfig, updateFn func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error) HealthChecker {
	return mcpclient.NewHealthChecker(manager, cfg, updateFn)
}

func defaultRuntimePortsFactory(cfg config.Config, manager *mcpclient.Manager, runtimeStore service.RuntimeStore) RuntimePorts {
	if manager != nil {
		adapter := service.NewLocalRuntimeAdapter(manager, runtimeStore)
		return RuntimePorts{
			Connector:    adapter,
			StatusReader: adapter,
			ToolCatalog:  adapter,
			ToolInvoker:  adapter,
		}
	}

	if resolveAppRole(cfg.App.Role) == appRoleControlPlane && cfg.RPC.Enabled && cfg.RPC.ExecutorTarget != "" {
		httpClient := &http.Client{Timeout: cfg.RPC.RequestTimeout}
		client := rpc.NewClient(cfg.RPC.ExecutorTarget, rpc.WithHTTPClient(httpClient))
		adapter := service.NewRemoteRuntimeAdapter(
			client,
			service.WithRemoteRuntimeSnapshotStore(runtimeStore),
			service.WithRemoteRuntimeStatusTimeout(cfg.RPC.RequestTimeout),
		)
		return RuntimePorts{
			Connector:    adapter,
			StatusReader: adapter,
			ToolCatalog:  adapter,
			ToolInvoker:  adapter,
		}
	}

	return RuntimePorts{}
}

func newHealthUpdateFn(serviceRepo repository.MCPServiceRepository, manager serviceDisconnecter, auditSink service.AuditSink, alertSvc service.AlertService) func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error {
	return func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error {
		item, err := serviceRepo.GetByID(ctx, serviceID)
		if err != nil {
			return err
		}
		prevStatus := item.Status
		if err := serviceRepo.UpdateStatus(ctx, serviceID, status, failureCount, lastError); err != nil {
			return err
		}
		if status != entity.ServiceStatusError {
			return nil
		}
		_ = manager.Disconnect(context.Background(), serviceID)
		if prevStatus == entity.ServiceStatusError {
			return nil
		}
		_ = auditSink.Record(ctx, service.AuditEntry{
			Username:     "system",
			Action:       "service_error",
			ResourceType: "mcp_service",
			ResourceID:   serviceID,
			Detail: map[string]any{
				"service_name":     item.Name,
				"transport_type":   item.TransportType,
				"status":           status,
				"failure_count":    failureCount,
				"reason":           lastError,
				"source":           "health_check",
				"listen_enabled":   item.ListenEnabled,
				"service_endpoint": endpointOf(item),
			},
		})
		_ = alertSvc.NotifyServiceError(ctx, item.Name, string(item.TransportType), endpointOf(item), lastError)
		return nil
	}
}

func endpointOf(serviceItem *entity.MCPService) string {
	if serviceItem == nil {
		return ""
	}
	if serviceItem.URL != "" {
		return serviceItem.URL
	}
	return serviceItem.Command
}

func newRedisClient(cfg config.RedisConfig) redis.UniversalClient {
	return redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})
}

func buildTokenBlacklistStore(client redis.UniversalClient, cfg config.Config) appcrypto.TokenBlacklistStore {
	if client == nil {
		return appcrypto.NewInMemoryTokenBlacklistStore()
	}
	return appcrypto.NewRedisTokenBlacklistStore(client, appcrypto.RedisBlacklistOptions{
		KeyPrefix:        cfg.Redis.KeyPrefix,
		OperationTimeout: cfg.Redis.OperationTimeout,
	})
}

func buildUserTokenVersionStore(client redis.UniversalClient, cfg config.Config) service.UserTokenVersionStore {
	if client == nil {
		return service.NoopUserTokenVersionStore{}
	}
	return service.NewRedisUserTokenVersionStore(client, service.AuthStateStoreOptions{
		KeyPrefix:        cfg.Redis.KeyPrefix,
		OperationTimeout: cfg.Redis.OperationTimeout,
	})
}

func buildSessionStateStore(client redis.UniversalClient, cfg config.Config) service.SessionStateStore {
	if client == nil {
		return service.NoopSessionStateStore{}
	}
	return service.NewRedisSessionStateStore(client, service.AuthStateStoreOptions{
		KeyPrefix:        cfg.Redis.KeyPrefix,
		OperationTimeout: cfg.Redis.OperationTimeout,
	})
}

func buildRuntimeStore(client redis.UniversalClient, cfg config.Config) service.RuntimeStore {
	if client == nil || !cfg.Runtime.SnapshotEnabled {
		return service.NoopRuntimeStore{}
	}
	return service.NewRedisRuntimeStore(client, service.RedisRuntimeStoreOptions{
		KeyPrefix:        cfg.Redis.KeyPrefix,
		SnapshotTTL:      cfg.Runtime.SnapshotTTL,
		OperationTimeout: cfg.Redis.OperationTimeout,
	})
}

func shouldStartupReconcile(cfg config.RuntimeConfig) bool {
	if cfg == (config.RuntimeConfig{}) {
		return true
	}
	return cfg.StartupReconcile
}

func runtimeSchemaReady(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	return db.Migrator().HasTable(&entity.MCPService{})
}

func buildReadinessProbe(role appRole, db *gorm.DB, cfg config.Config, ports RuntimePorts) router.ReadinessProbe {
	return func(ctx context.Context) router.ReadinessStatus {
		status := router.ReadinessStatus{
			Role:  string(role),
			Ready: true,
			Checks: map[string]router.ReadinessCheck{
				"http": {Ready: true},
			},
			Reason: "ready",
		}

		dbReady, dbReason := probeDatabase(ctx, db)
		status.Checks["database"] = router.ReadinessCheck{Ready: dbReady, Reason: dbReason}
		if !dbReady {
			status.Ready = false
			status.Reason = "database unavailable"
		}

		switch role {
		case appRoleControlPlane:
			rpcReady, rpcReason := probeExecutorRPC(ctx, cfg)
			status.Checks["executor_rpc"] = router.ReadinessCheck{Ready: rpcReady, Reason: rpcReason}
			if !rpcReady {
				status.Ready = false
				status.Reason = "executor rpc unavailable"
			}
		case appRoleExecutor:
			runtimeReady := ports.Connector != nil && ports.StatusReader != nil && ports.ToolCatalog != nil && ports.ToolInvoker != nil
			status.Checks["runtime"] = router.ReadinessCheck{Ready: runtimeReady, Reason: readinessReason(runtimeReady, "runtime ready", "runtime ports missing")}
			status.Checks["rpc_server"] = router.ReadinessCheck{Ready: cfg.RPC.Enabled, Reason: readinessReason(cfg.RPC.Enabled, "rpc server enabled", "rpc disabled")}
			if !runtimeReady || !cfg.RPC.Enabled {
				status.Ready = false
				status.Reason = "executor dependencies unavailable"
			}
		default:
			runtimeReady := ports.Connector != nil && ports.StatusReader != nil && ports.ToolCatalog != nil && ports.ToolInvoker != nil
			status.Checks["runtime"] = router.ReadinessCheck{Ready: runtimeReady, Reason: readinessReason(runtimeReady, "runtime ready", "runtime ports missing")}
			if !runtimeReady {
				status.Ready = false
				status.Reason = "runtime unavailable"
			}
		}

		return status
	}
}

func probeDatabase(ctx context.Context, db *gorm.DB) (bool, string) {
	if db == nil {
		return false, "db is nil"
	}
	sqlDB, err := db.DB()
	if err != nil {
		return false, err.Error()
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return false, err.Error()
	}
	return true, "database reachable"
}

func probeExecutorRPC(ctx context.Context, cfg config.Config) (bool, string) {
	if !cfg.RPC.Enabled {
		return false, "rpc disabled"
	}
	if cfg.RPC.ExecutorTarget == "" {
		return false, "executor target missing"
	}
	httpClient := &http.Client{Timeout: cfg.RPC.RequestTimeout}
	client := rpc.NewClient(cfg.RPC.ExecutorTarget, rpc.WithHTTPClient(httpClient))
	if err := client.PingExecutor(ctx); err != nil {
		return false, err.Error()
	}
	return true, "executor reachable"
}

func readinessReason(ready bool, successReason, failureReason string) string {
	if ready {
		return successReason
	}
	return failureReason
}

type rpcExecutorAdapter struct {
	ports RuntimePorts
}

func newRPCExecutor(ports RuntimePorts) rpc.Executor {
	return &rpcExecutorAdapter{ports: ports}
}

func (a *rpcExecutorAdapter) Connect(ctx context.Context, serviceItem *entity.MCPService) (mcpclient.RuntimeStatus, error) {
	return a.ports.Connector.Connect(ctx, serviceItem)
}

func (a *rpcExecutorAdapter) Disconnect(ctx context.Context, serviceID string) error {
	return a.ports.Connector.Disconnect(ctx, serviceID)
}

func (a *rpcExecutorAdapter) GetStatus(_ context.Context, serviceID string) (mcpclient.RuntimeStatus, bool, error) {
	status, ok := a.ports.StatusReader.GetStatus(serviceID)
	return status, ok, nil
}

func (a *rpcExecutorAdapter) ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error) {
	return a.ports.ToolCatalog.ListTools(ctx, serviceID)
}

func (a *rpcExecutorAdapter) CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	return a.ports.ToolInvoker.CallTool(ctx, serviceID, name, args)
}

func (a *rpcExecutorAdapter) Ping(context.Context) error {
	return nil
}

func reconcileStartupStatuses(ctx context.Context, serviceRepo repository.MCPServiceRepository) error {
	_, err := serviceRepo.ResetConnectionStatuses(ctx)
	return err
}
