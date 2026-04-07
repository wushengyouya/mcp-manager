package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/database"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/handler"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/router"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/internal/task"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/email"
	"github.com/mikasa/mcp-manager/scripts"
)

type serviceDisconnecter interface {
	Disconnect(ctx context.Context, serviceID string) error
}

// RuntimeFactory 定义运行时管理器构造函数。
type RuntimeFactory func(appCfg config.AppConfig) *mcpclient.Manager

// JWTFactory 定义 JWT 服务构造函数。
type JWTFactory func(cfg config.JWTConfig) *appcrypto.JWTService

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

// App 保存应用装配产物，供主程序与测试复用。
type App struct {
	Engine  *gin.Engine
	Server  *http.Server
	Cleanup func()
}

// Builder 定义应用装配器。
type Builder struct {
	cfg                     config.Config
	runtimeFactory          RuntimeFactory
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

	if err := database.Migrate(db); err != nil {
		cleanup()
		return nil, fmt.Errorf("执行迁移失败: %w", err)
	}

	userRepo := repository.NewUserRepository(db)
	serviceRepo := repository.NewMCPServiceRepository(db)
	toolRepo := repository.NewToolRepository(db)
	historyRepo := repository.NewRequestHistoryRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	if err := scripts.EnsureAdmin(context.Background(), userRepo, b.cfg.App.InitAdminUsername, b.cfg.App.InitAdminPassword, b.cfg.App.InitAdminEmail); err != nil {
		cleanup()
		return nil, fmt.Errorf("初始化管理员失败: %w", err)
	}
	if err := reconcileStartupStatuses(context.Background(), serviceRepo); err != nil {
		cleanup()
		return nil, fmt.Errorf("修正启动状态失败: %w", err)
	}

	jwtSvc := b.jwtFactory(b.cfg.JWT)
	auditSink := b.auditSinkFactory(auditRepo)
	authSvc := service.NewAuthService(userRepo, jwtSvc, auditSink)
	userSvc := service.NewUserService(userRepo, auditSink)
	manager := b.runtimeFactory(b.cfg.App)
	auditSvc := service.NewAuditService(auditSink, auditRepo)
	runtimeAdapter := service.NewLocalRuntimeAdapter(manager)

	var sender email.Sender
	if b.cfg.Alert.Enabled {
		sender = email.NewSMTPSender(b.cfg.Alert.SMTPHost, b.cfg.Alert.SMTPPort, b.cfg.Alert.SMTPUsername, b.cfg.Alert.SMTPPassword)
	}
	alertSvc := service.NewAlertService(b.cfg.Alert, sender)
	mcpSvc := service.NewMCPService(serviceRepo, toolRepo, runtimeAdapter, runtimeAdapter, auditSink, alertSvc)
	toolSvc := service.NewToolService(toolRepo, serviceRepo, runtimeAdapter, auditSink)
	invokeSvc := service.NewToolInvokeService(b.cfg.History, toolRepo, serviceRepo, historyRepo, runtimeAdapter)

	if b.cfg.HealthCheck.Enabled {
		healthChecker := b.healthCheckerFactory(manager, b.cfg.HealthCheck, newHealthUpdateFn(serviceRepo, manager, auditSink, alertSvc))
		healthChecker.Start()
		cleanupFns = append(cleanupFns, healthChecker.Stop)
	}

	cleanupTask := b.auditCleanupTaskFactory(auditRepo, b.cfg.Audit.RetentionDays, b.cfg.Audit.CleanupInterval)
	cleanupTask.Start()
	cleanupFns = append(cleanupFns, cleanupTask.Stop)

	engine := router.New(jwtSvc, router.Handlers{
		Auth:    handler.NewAuthHandler(authSvc),
		User:    handler.NewUserHandler(userSvc, authSvc),
		MCP:     handler.NewMCPHandler(mcpSvc),
		Tool:    handler.NewToolHandler(toolSvc, invokeSvc),
		History: handler.NewHistoryHandler(historyRepo),
		Audit:   handler.NewAuditHandler(auditSvc),
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", b.cfg.Server.Host, b.cfg.Server.Port),
		Handler:      engine,
		ReadTimeout:  b.cfg.Server.ReadTimeout,
		WriteTimeout: b.cfg.Server.WriteTimeout,
	}

	return &App{
		Engine:  engine,
		Server:  srv,
		Cleanup: cleanup,
	}, nil
}

func defaultJWTFactory(cfg config.JWTConfig) *appcrypto.JWTService {
	return appcrypto.NewJWTService(cfg.Secret, cfg.Issuer, cfg.AccessTTL, cfg.RefreshTTL, appcrypto.NewTokenBlacklist())
}

func defaultHealthCheckerFactory(manager *mcpclient.Manager, cfg config.HealthCheckConfig, updateFn func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error) HealthChecker {
	return mcpclient.NewHealthChecker(manager, cfg, updateFn)
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

func reconcileStartupStatuses(ctx context.Context, serviceRepo repository.MCPServiceRepository) error {
	_, err := serviceRepo.ResetConnectionStatuses(ctx)
	return err
}
