package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
)

// HealthChecker 定义健康检查器
type HealthChecker struct {
	manager          *Manager
	interval         time.Duration
	timeout          time.Duration
	failureThreshold int
	updateFn         func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error
	idsFn            func() []string
	pingFn           func(ctx context.Context, serviceID string) (RuntimeStatus, error)
	probeFn          func(ctx context.Context, serviceID string) (RuntimeStatus, error)
	syncRuntimeFn    func(serviceID string, status entity.ServiceStatus, failureCount int, lastError string)
	stop             chan struct{}
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(manager *Manager, cfg config.HealthCheckConfig, updateFn func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error) *HealthChecker {
	return &HealthChecker{
		manager:          manager,
		interval:         cfg.Interval,
		timeout:          cfg.Timeout,
		failureThreshold: cfg.FailureThreshold,
		updateFn:         updateFn,
		idsFn:            manager.IDs,
		pingFn:           manager.Ping,
		probeFn:          manager.ProbeTools,
		syncRuntimeFn:    manager.applyHealthState,
		stop:             make(chan struct{}),
	}
}

// Start 启动健康检查
func (h *HealthChecker) Start() {
	if h.interval <= 0 {
		h.interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.checkOnce()
			case <-h.stop:
				return
			}
		}
	}()
}

// checkOnce 对全部已连接服务执行一轮健康检查
func (h *HealthChecker) checkOnce() {
	var wg sync.WaitGroup
	for _, serviceID := range h.idsFn() {
		serviceID := serviceID
		wg.Go(func() {

			ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
			defer cancel()

			status, err := h.pingFn(ctx, serviceID)
			if err == nil {
				h.markHealthy(serviceID)
				return
			}

			// 部分服务未实现标准 ping，这里退化为 list_tools 探活，减少误报
			if isUnsupportedPingError(err) && h.probeFn != nil {
				pingErr := err
				if runtimeStatus, fallbackErr := h.probeFn(ctx, serviceID); fallbackErr == nil {
					_ = runtimeStatus
					h.markHealthy(serviceID)
					return
				} else {
					status = runtimeStatus
					h.markFailure(serviceID, status, fmt.Sprintf("unsupported_ping: %s; fallback failed: %s", pingErr.Error(), fallbackErr.Error()))
					return
				}
			}

			h.markFailure(serviceID, status, err)
		})
	}
	wg.Wait()
}

// Stop 停止健康检查
func (h *HealthChecker) Stop() {
	select {
	case <-h.stop:
	default:
		close(h.stop)
	}
}

// markHealthy 将服务状态重置为健康
func (h *HealthChecker) markHealthy(serviceID string) {
	if h.syncRuntimeFn != nil {
		h.syncRuntimeFn(serviceID, entity.ServiceStatusConnected, 0, "")
	}
	if h.updateFn != nil {
		_ = h.updateFn(context.Background(), serviceID, entity.ServiceStatusConnected, 0, "")
	}
}

// markFailure 根据失败次数推进服务状态并落库
func (h *HealthChecker) markFailure(serviceID string, status RuntimeStatus, failure any) {
	next := status.FailureCount + 1
	svcStatus := entity.ServiceStatusConnected
	if err, ok := failure.(error); ok && IsSessionReconnectRequired(err) {
		next = h.failureThreshold
		svcStatus = entity.ServiceStatusError
	} else if next >= h.failureThreshold {
		svcStatus = entity.ServiceStatusError
	}
	lastError := formatHealthFailure(failure)
	if h.syncRuntimeFn != nil {
		h.syncRuntimeFn(serviceID, svcStatus, next, lastError)
	}
	if h.updateFn != nil {
		_ = h.updateFn(context.Background(), serviceID, svcStatus, next, lastError)
	}
}

// formatHealthFailure 将失败原因统一格式化为可读文本
func formatHealthFailure(failure any) string {
	switch v := failure.(type) {
	case nil:
		return ""
	case string:
		return v
	case error:
		return fmt.Sprintf("%s: %s", classifyHealthError(v), v.Error())
	default:
		return fmt.Sprint(v)
	}
}

// classifyHealthError 对健康检查错误进行分类
func classifyHealthError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrServiceNotConnected) {
		return "disconnected"
	}
	if IsSessionReconnectRequired(err) {
		return "session_expired"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	return "invoke_failed"
}

// isUnsupportedPingError 判断错误是否表示服务端不支持 ping
func isUnsupportedPingError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "method not found") ||
		strings.Contains(message, "-32601") ||
		strings.Contains(message, "not supported")
}
