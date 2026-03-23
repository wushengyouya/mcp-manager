package mcpclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

func TestHealthCheckerCheckOnceAccumulatesFailures(t *testing.T) {
	manager := NewManager(config.AppConfig{})
	manager.items["svc-1"] = &managedClient{
		service: &entity.MCPService{ListenEnabled: true},
		runtime: RuntimeStatus{
			ServiceID:     "svc-1",
			Status:        entity.ServiceStatusConnected,
			ListenEnabled: true,
			ListenActive:  true,
		},
	}

	type update struct {
		status       entity.ServiceStatus
		failureCount int
		lastError    string
	}
	var updates []update

	checker := NewHealthChecker(manager, config.HealthCheckConfig{
		Interval:         time.Second,
		Timeout:          time.Second,
		FailureThreshold: 3,
	}, func(_ context.Context, _ string, status entity.ServiceStatus, failureCount int, lastError string) error {
		updates = append(updates, update{status: status, failureCount: failureCount, lastError: lastError})
		return nil
	})
	checker.idsFn = func() []string { return []string{"svc-1"} }
	checker.pingFn = func(_ context.Context, serviceID string) (RuntimeStatus, error) {
		status, ok := manager.GetStatus(serviceID)
		require.True(t, ok)
		return status, errors.New("ping failed")
	}

	checker.checkOnce()
	checker.checkOnce()
	checker.checkOnce()

	status, ok := manager.GetStatus("svc-1")
	require.True(t, ok)
	require.Equal(t, entity.ServiceStatusError, status.Status)
	require.Equal(t, 3, status.FailureCount)
	require.Equal(t, "invoke_failed: ping failed", status.LastError)
	require.False(t, status.ListenActive)

	require.Len(t, updates, 3)
	require.Equal(t, entity.ServiceStatusConnected, updates[0].status)
	require.Equal(t, 1, updates[0].failureCount)
	require.Equal(t, entity.ServiceStatusConnected, updates[1].status)
	require.Equal(t, 2, updates[1].failureCount)
	require.Equal(t, entity.ServiceStatusError, updates[2].status)
	require.Equal(t, 3, updates[2].failureCount)
}

func TestHealthCheckerCheckOnceResetsStateOnRecovery(t *testing.T) {
	manager := NewManager(config.AppConfig{})
	manager.items["svc-1"] = &managedClient{
		service: &entity.MCPService{ListenEnabled: true},
		runtime: RuntimeStatus{
			ServiceID:     "svc-1",
			Status:        entity.ServiceStatusError,
			ListenEnabled: true,
			ListenActive:  false,
			LastError:     "ping failed",
			FailureCount:  2,
		},
	}

	type update struct {
		status       entity.ServiceStatus
		failureCount int
		lastError    string
	}
	var updates []update

	checker := NewHealthChecker(manager, config.HealthCheckConfig{
		Interval:         time.Second,
		Timeout:          time.Second,
		FailureThreshold: 3,
	}, func(_ context.Context, _ string, status entity.ServiceStatus, failureCount int, lastError string) error {
		updates = append(updates, update{status: status, failureCount: failureCount, lastError: lastError})
		return nil
	})
	checker.idsFn = func() []string { return []string{"svc-1"} }
	checker.pingFn = func(_ context.Context, serviceID string) (RuntimeStatus, error) {
		status, ok := manager.GetStatus(serviceID)
		require.True(t, ok)
		return status, nil
	}

	checker.checkOnce()

	status, ok := manager.GetStatus("svc-1")
	require.True(t, ok)
	require.Equal(t, entity.ServiceStatusConnected, status.Status)
	require.Equal(t, 0, status.FailureCount)
	require.Empty(t, status.LastError)
	require.True(t, status.ListenActive)
	require.Empty(t, status.ListenLastError)

	require.Len(t, updates, 1)
	require.Equal(t, entity.ServiceStatusConnected, updates[0].status)
	require.Equal(t, 0, updates[0].failureCount)
	require.Empty(t, updates[0].lastError)
}

func TestHealthCheckerCheckOnceFallbacksWhenPingUnsupported(t *testing.T) {
	manager := NewManager(config.AppConfig{})
	manager.items["svc-1"] = &managedClient{
		service: &entity.MCPService{ListenEnabled: true},
		runtime: RuntimeStatus{
			ServiceID:     "svc-1",
			Status:        entity.ServiceStatusConnected,
			ListenEnabled: true,
			ListenActive:  true,
		},
	}

	var fallbackCalled bool
	checker := NewHealthChecker(manager, config.HealthCheckConfig{
		Interval:         time.Second,
		Timeout:          time.Second,
		FailureThreshold: 3,
	}, nil)
	checker.idsFn = func() []string { return []string{"svc-1"} }
	checker.pingFn = func(_ context.Context, serviceID string) (RuntimeStatus, error) {
		status, ok := manager.GetStatus(serviceID)
		require.True(t, ok)
		return status, errors.New("Method not found")
	}
	checker.listToolsFn = func(_ context.Context, serviceID string) ([]mcp.Tool, RuntimeStatus, error) {
		fallbackCalled = true
		status, ok := manager.GetStatus(serviceID)
		require.True(t, ok)
		return nil, status, nil
	}

	checker.checkOnce()

	status, ok := manager.GetStatus("svc-1")
	require.True(t, ok)
	require.True(t, fallbackCalled)
	require.Equal(t, entity.ServiceStatusConnected, status.Status)
	require.Zero(t, status.FailureCount)
	require.Empty(t, status.LastError)
}

func TestHealthCheckerCheckOnceRunsServicesConcurrently(t *testing.T) {
	manager := NewManager(config.AppConfig{})
	manager.items["slow"] = &managedClient{service: &entity.MCPService{}, runtime: RuntimeStatus{ServiceID: "slow", Status: entity.ServiceStatusConnected}}
	manager.items["fast"] = &managedClient{service: &entity.MCPService{}, runtime: RuntimeStatus{ServiceID: "fast", Status: entity.ServiceStatusConnected}}

	checker := NewHealthChecker(manager, config.HealthCheckConfig{
		Interval:         time.Second,
		Timeout:          time.Second,
		FailureThreshold: 3,
	}, nil)
	checker.idsFn = func() []string { return []string{"slow", "fast"} }

	releaseSlow := make(chan struct{})
	checker.pingFn = func(_ context.Context, serviceID string) (RuntimeStatus, error) {
		status, ok := manager.GetStatus(serviceID)
		require.True(t, ok)
		if serviceID == "slow" {
			<-releaseSlow
			return status, nil
		}
		close(releaseSlow)
		return status, nil
	}

	done := make(chan struct{})
	go func() {
		checker.checkOnce()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("health checker should inspect services concurrently")
	}
}
