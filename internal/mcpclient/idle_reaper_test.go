package mcpclient

import (
	"context"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

func TestEvaluateIdleReaperDryRun(t *testing.T) {
	now := time.Now()
	oldUsedAt := now.Add(-3 * time.Minute)

	tests := []struct {
		name       string
		cfg        config.RuntimeConfig
		status     RuntimeStatus
		wouldReap  bool
		skipReason IdleReaperSkipReason
	}{
		{
			name:       "feature disabled",
			cfg:        config.RuntimeConfig{IdleReaperDryRunEnabled: false, IdleTimeout: time.Minute},
			status:     RuntimeStatus{ServiceID: "svc-1", Status: entity.ServiceStatusConnected, LastUsedAt: &oldUsedAt},
			skipReason: IdleReaperSkipReasonFeatureDisabled,
		},
		{
			name:       "idle timeout disabled",
			cfg:        config.RuntimeConfig{IdleReaperDryRunEnabled: true, IdleTimeout: 0},
			status:     RuntimeStatus{ServiceID: "svc-1", Status: entity.ServiceStatusConnected, LastUsedAt: &oldUsedAt},
			skipReason: IdleReaperSkipReasonIdleTimeoutDisabled,
		},
		{
			name:       "in flight",
			cfg:        config.RuntimeConfig{IdleReaperDryRunEnabled: true, IdleTimeout: time.Minute},
			status:     RuntimeStatus{ServiceID: "svc-1", Status: entity.ServiceStatusConnected, LastUsedAt: &oldUsedAt, InFlight: 1},
			skipReason: IdleReaperSkipReasonInFlight,
		},
		{
			name:       "listen enabled",
			cfg:        config.RuntimeConfig{IdleReaperDryRunEnabled: true, IdleTimeout: time.Minute},
			status:     RuntimeStatus{ServiceID: "svc-1", Status: entity.ServiceStatusConnected, LastUsedAt: &oldUsedAt, ListenEnabled: true},
			skipReason: IdleReaperSkipReasonListenEnabled,
		},
		{
			name:       "recently used",
			cfg:        config.RuntimeConfig{IdleReaperDryRunEnabled: true, IdleTimeout: time.Minute},
			status:     RuntimeStatus{ServiceID: "svc-1", Status: entity.ServiceStatusConnected, LastUsedAt: ptrTime(now.Add(-20 * time.Second))},
			skipReason: IdleReaperSkipReasonRecentlyUsed,
		},
		{
			name:       "not connected",
			cfg:        config.RuntimeConfig{IdleReaperDryRunEnabled: true, IdleTimeout: time.Minute},
			status:     RuntimeStatus{ServiceID: "svc-1", Status: entity.ServiceStatusError, LastUsedAt: &oldUsedAt},
			skipReason: IdleReaperSkipReasonNotConnected,
		},
		{
			name:      "would reap",
			cfg:       config.RuntimeConfig{IdleReaperDryRunEnabled: true, IdleTimeout: time.Minute},
			status:    RuntimeStatus{ServiceID: "svc-1", Status: entity.ServiceStatusConnected, LastUsedAt: &oldUsedAt},
			wouldReap: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observation := EvaluateIdleReaperDryRun(tt.status, tt.cfg, now)
			require.Equal(t, tt.wouldReap, observation.WouldReap)
			require.Equal(t, tt.skipReason, observation.SkipReason)
		})
	}
}

func TestManagerScanIdleReaperDryRun(t *testing.T) {
	now := time.Now()
	oldUsedAt := now.Add(-2 * time.Minute)
	manager := NewManager(config.AppConfig{})
	manager.items["svc-idle"] = &managedClient{
		service: &entity.MCPService{Base: entity.Base{ID: "svc-idle"}},
		runtime: RuntimeStatus{
			ServiceID:  "svc-idle",
			Status:     entity.ServiceStatusConnected,
			LastUsedAt: &oldUsedAt,
		},
	}
	manager.items["svc-busy"] = &managedClient{
		service: &entity.MCPService{Base: entity.Base{ID: "svc-busy"}},
		runtime: RuntimeStatus{
			ServiceID:  "svc-busy",
			Status:     entity.ServiceStatusConnected,
			LastUsedAt: &oldUsedAt,
			InFlight:   2,
		},
	}

	var observations []IdleReaperDryRunObservation
	manager.ScanIdleReaperDryRun(now, config.RuntimeConfig{
		IdleReaperDryRunEnabled: true,
		IdleTimeout:             time.Minute,
	}, func(observation IdleReaperDryRunObservation) {
		observations = append(observations, observation)
	})

	require.Len(t, observations, 2)
	results := map[string]IdleReaperDryRunObservation{}
	for _, observation := range observations {
		results[observation.ServiceID] = observation
	}
	require.True(t, results["svc-idle"].WouldReap)
	require.Equal(t, IdleReaperSkipReasonInFlight, results["svc-busy"].SkipReason)
}

func TestManagerPingDoesNotAffectIdleDryRunDecision(t *testing.T) {
	lastUsedAt := time.Now().Add(-2 * time.Minute)
	fake := &fakeRuntimeClient{sessionID: "sess-1"}
	manager := NewManager(config.AppConfig{})
	manager.items["svc-ping"] = &managedClient{
		service: &entity.MCPService{Base: entity.Base{ID: "svc-ping"}},
		client:  fake,
		runtime: RuntimeStatus{
			ServiceID:       "svc-ping",
			Status:          entity.ServiceStatusConnected,
			LastUsedAt:      &lastUsedAt,
			SessionIDExists: true,
		},
	}

	cfg := config.RuntimeConfig{IdleReaperDryRunEnabled: true, IdleTimeout: time.Minute}
	before := captureIdleObservation(t, manager, cfg, "svc-ping")
	require.True(t, before.WouldReap)

	_, err := manager.Ping(context.Background(), "svc-ping")
	require.NoError(t, err)

	after := captureIdleObservation(t, manager, cfg, "svc-ping")
	require.True(t, after.WouldReap)
	require.Equal(t, before.LastUsedAt, after.LastUsedAt)
}

func TestIdleReaperDryRunScannerCheckOnceEmitsObservation(t *testing.T) {
	now := time.Now()
	lastUsedAt := now.Add(-3 * time.Minute)
	manager := NewManager(config.AppConfig{})
	manager.items["svc-scan"] = &managedClient{
		service: &entity.MCPService{Base: entity.Base{ID: "svc-scan"}},
		runtime: RuntimeStatus{
			ServiceID:  "svc-scan",
			Status:     entity.ServiceStatusConnected,
			LastUsedAt: &lastUsedAt,
		},
	}

	scanner := NewIdleReaperDryRunScanner(manager, config.RuntimeConfig{
		IdleReaperDryRunEnabled: true,
		IdleTimeout:             time.Minute,
	})
	scanner.nowFn = func() time.Time { return now }
	var observations []IdleReaperDryRunObservation
	scanner.observeFn = func(observation IdleReaperDryRunObservation) {
		observations = append(observations, observation)
	}

	scanner.checkOnce()

	require.Len(t, observations, 1)
	require.True(t, observations[0].WouldReap)
	require.Equal(t, "svc-scan", observations[0].ServiceID)
}

func captureIdleObservation(t *testing.T, manager *Manager, cfg config.RuntimeConfig, serviceID string) IdleReaperDryRunObservation {
	t.Helper()
	var got IdleReaperDryRunObservation
	manager.ScanIdleReaperDryRun(time.Now(), cfg, func(observation IdleReaperDryRunObservation) {
		if observation.ServiceID == serviceID {
			got = observation
		}
	})
	require.Equal(t, serviceID, got.ServiceID)
	return got
}

func ptrTime(v time.Time) *time.Time {
	return &v
}
