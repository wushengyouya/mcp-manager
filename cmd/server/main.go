package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mikasa/mcp-manager/api/docs"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/database"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/handler"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/router"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/internal/task"
	"github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/email"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/mikasa/mcp-manager/scripts"
	"go.uber.org/zap"
)

// @title MCP 服务管理平台
// @version 0.1.0
// @description MCP 服务管理平台 API
// @BasePath /api/v1
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description 输入 Bearer Token，例如 `Bearer eyJ...`
func main() {
	cfg, err := config.Load(".")
	if err != nil {
		panic(err)
	}
	if err := logger.Init(cfg.Log); err != nil {
		panic(err)
	}

	srv, cleanup, err := buildApp(*cfg)
	if err != nil {
		logger.L().Fatal("构建应用失败", zapError(err))
	}
	defer cleanup()

	go func() {
		logger.S().Infof("server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.L().Fatal("启动 HTTP 服务失败", zapError(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.L().Error("关闭 HTTP 服务失败", zapError(err))
	}
}

type serviceDisconnecter interface {
	Disconnect(ctx context.Context, serviceID string) error
}

func newHealthUpdateFn(serviceRepo repository.MCPServiceRepository, manager serviceDisconnecter, auditSink service.AuditSink, alertSvc service.AlertService) func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error {
	return func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error {
		item, err := serviceRepo.GetByID(ctx, serviceID)
		if err != nil {
			return err
		}
		prevStatus := item.Status
		// 先持久化最新健康状态，避免内存状态与数据库状态不一致
		if err := serviceRepo.UpdateStatus(ctx, serviceID, status, failureCount, lastError); err != nil {
			return err
		}
		if status != entity.ServiceStatusError {
			return nil
		}
		// 健康检查判定异常后主动断开连接，防止后续继续使用失效连接
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

func buildApp(cfg config.Config) (*http.Server, func(), error) {
	db, err := database.Init(cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("初始化数据库失败: %w", err)
	}

	cleanupFns := []func(){func() { _ = database.Close() }}
	cleanup := func() {
		for i := len(cleanupFns) - 1; i >= 0; i-- {
			cleanupFns[i]()
		}
	}

	if err := database.Migrate(db); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("执行迁移失败: %w", err)
	}

	userRepo := repository.NewUserRepository(db)
	serviceRepo := repository.NewMCPServiceRepository(db)
	toolRepo := repository.NewToolRepository(db)
	historyRepo := repository.NewRequestHistoryRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	if err := scripts.EnsureAdmin(context.Background(), userRepo, cfg.App.InitAdminUsername, cfg.App.InitAdminPassword, cfg.App.InitAdminEmail); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("初始化管理员失败: %w", err)
	}

	blacklist := crypto.NewTokenBlacklist()
	jwtSvc := crypto.NewJWTService(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL, blacklist)
	auditSink := service.NewDBAuditSink(auditRepo)
	authSvc := service.NewAuthService(userRepo, jwtSvc, auditSink)
	userSvc := service.NewUserService(userRepo, auditSink)
	manager := mcpclient.NewManager(cfg.App)
	auditSvc := service.NewAuditService(auditSink, auditRepo)

	var sender email.Sender
	if cfg.Alert.Enabled {
		sender = email.NewSMTPSender(cfg.Alert.SMTPHost, cfg.Alert.SMTPPort, cfg.Alert.SMTPUsername, cfg.Alert.SMTPPassword)
	}
	alertSvc := service.NewAlertService(cfg.Alert, sender)
	mcpSvc := service.NewMCPService(serviceRepo, toolRepo, manager, auditSink, alertSvc)
	toolSvc := service.NewToolService(toolRepo, serviceRepo, manager, auditSink)
	invokeSvc := service.NewToolInvokeService(cfg.History, toolRepo, serviceRepo, historyRepo, manager)

	if cfg.HealthCheck.Enabled {
		healthChecker := mcpclient.NewHealthChecker(manager, cfg.HealthCheck, newHealthUpdateFn(serviceRepo, manager, auditSink, alertSvc))
		healthChecker.Start()
		cleanupFns = append(cleanupFns, healthChecker.Stop)
	}

	cleanupTask := task.NewAuditCleanupTask(auditRepo, cfg.Audit.RetentionDays, cfg.Audit.CleanupInterval)
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
		Addr:         cfg.Server.Host + ":" + itoa(cfg.Server.Port),
		Handler:      engine,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}
	return srv, cleanup, nil
}

// itoa 将端口等整数转换为字符串
func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}

// zapError 将错误包装为 zap 字段
func zapError(err error) zap.Field {
	return zap.Error(err)
}

// endpointOf 返回服务的主要访问端点
func endpointOf(serviceItem *entity.MCPService) string {
	if serviceItem == nil {
		return ""
	}
	if serviceItem.URL != "" {
		return serviceItem.URL
	}
	return serviceItem.Command
}
