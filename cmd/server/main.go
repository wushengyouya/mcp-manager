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

	db, err := database.Init(cfg.Database)
	if err != nil {
		logger.L().Fatal("初始化数据库失败", zapError(err))
	}
	defer database.Close()

	if err := database.Migrate(db); err != nil {
		logger.L().Fatal("执行迁移失败", zapError(err))
	}

	userRepo := repository.NewUserRepository(db)
	serviceRepo := repository.NewMCPServiceRepository(db)
	toolRepo := repository.NewToolRepository(db)
	historyRepo := repository.NewRequestHistoryRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	if err := scripts.EnsureAdmin(context.Background(), userRepo, cfg.App.InitAdminUsername, cfg.App.InitAdminPassword, cfg.App.InitAdminEmail); err != nil {
		logger.L().Fatal("初始化管理员失败", zapError(err))
	}

	blacklist := crypto.NewTokenBlacklist()
	jwtSvc := crypto.NewJWTService(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL, blacklist)
	auditSink := service.NewDBAuditSink(auditRepo)
	authSvc := service.NewAuthService(userRepo, jwtSvc, auditSink)
	userSvc := service.NewUserService(userRepo, auditSink)
	manager := mcpclient.NewManager(cfg.App)
	mcpSvc := service.NewMCPService(serviceRepo, manager, auditSink)
	toolSvc := service.NewToolService(toolRepo, serviceRepo, manager, auditSink)
	invokeSvc := service.NewToolInvokeService(cfg.History, toolRepo, serviceRepo, historyRepo, manager)
	auditSvc := service.NewAuditService(auditSink, auditRepo)

	var sender email.Sender
	if cfg.Alert.Enabled {
		sender = email.NewSMTPSender(cfg.Alert.SMTPHost, cfg.Alert.SMTPPort, cfg.Alert.SMTPUsername, cfg.Alert.SMTPPassword)
	}
	_ = service.NewAlertService(cfg.Alert, sender)

	healthChecker := mcpclient.NewHealthChecker(manager, cfg.HealthCheck, serviceRepo.UpdateStatus)
	if cfg.HealthCheck.Enabled {
		healthChecker.Start()
		defer healthChecker.Stop()
	}

	cleanupTask := task.NewAuditCleanupTask(auditRepo, cfg.Audit.RetentionDays, cfg.Audit.CleanupInterval)
	cleanupTask.Start()
	defer cleanupTask.Stop()

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

func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}

func zapError(err error) zap.Field {
	return zap.Error(err)
}
