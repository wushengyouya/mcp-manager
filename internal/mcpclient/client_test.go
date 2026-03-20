package mcpclient

import (
	"context"
	"errors"
	"testing"
	"time"

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
	require.Equal(t, "ping failed", status.LastError)
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
