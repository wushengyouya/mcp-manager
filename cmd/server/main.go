package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mikasa/mcp-manager/api/docs"
	"github.com/mikasa/mcp-manager/internal/bootstrap"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/logger"
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

// newHealthUpdateFn 创建健康检查状态更新回调。
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

// buildApp 构建应用依赖并返回 HTTP 服务与清理函数。
func buildApp(cfg config.Config) (*http.Server, func(), error) {
	app, err := bootstrap.NewBuilder(cfg).Build()
	if err != nil {
		return nil, nil, err
	}
	return app.Server, app.Cleanup, nil
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
