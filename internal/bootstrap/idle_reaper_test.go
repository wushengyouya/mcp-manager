package bootstrap

import (
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/stretchr/testify/require"
)

type idleReaperStub struct {
	started bool
	stopped bool
}

func (s *idleReaperStub) Start() { s.started = true }
func (s *idleReaperStub) Stop()  { s.stopped = true }

func TestBuilderIdleReaperOnlyRunsOnLocalRuntimeWhenEnabled(t *testing.T) {
	require.NoError(t, logger.Init(config.LogConfig{Level: "info", Format: "console", Output: "stdout"}))

	tests := []struct {
		name        string
		role        string
		cfg         config.RuntimeConfig
		expectStart bool
		expectStop  bool
	}{
		{
			name:        "all role enabled",
			role:        "all",
			cfg:         config.RuntimeConfig{IdleReaperDryRunEnabled: true, IdleTimeout: time.Minute},
			expectStart: true,
			expectStop:  true,
		},
		{
			name:        "executor role enabled",
			role:        "executor",
			cfg:         config.RuntimeConfig{IdleReaperDryRunEnabled: true, IdleTimeout: time.Minute},
			expectStart: true,
			expectStop:  true,
		},
		{
			name:        "control-plane never starts",
			role:        "control-plane",
			cfg:         config.RuntimeConfig{IdleReaperDryRunEnabled: true, IdleTimeout: time.Minute},
			expectStart: false,
			expectStop:  false,
		},
		{
			name:        "feature disabled stays off",
			role:        "all",
			cfg:         config.RuntimeConfig{IdleReaperDryRunEnabled: false, IdleTimeout: time.Minute},
			expectStart: false,
			expectStop:  false,
		},
		{
			name:        "timeout zero stays off",
			role:        "all",
			cfg:         config.RuntimeConfig{IdleReaperDryRunEnabled: true, IdleTimeout: 0},
			expectStart: false,
			expectStop:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t)
			cfg.App.Role = tt.role
			cfg.Runtime = tt.cfg
			if tt.role == "control-plane" {
				cfg.RPC.Enabled = true
				cfg.RPC.ExecutorTarget = "http://127.0.0.1:18081"
				cfg.RPC.AuthToken = "token"
				cfg.RPC.RequestTimeout = time.Second
				cfg.RPC.DialTimeout = time.Second
			}
			if tt.role == "executor" {
				cfg.RPC.Enabled = true
				cfg.RPC.ListenAddr = "127.0.0.1:19081"
				cfg.RPC.AuthToken = "token"
				cfg.RPC.RequestTimeout = time.Second
				cfg.RPC.DialTimeout = time.Second
			}

			stub := &idleReaperStub{}
			called := false
			app, err := NewBuilder(cfg).
				WithIdleReaperFactory(func(_ *mcpclient.Manager, _ config.RuntimeConfig) IdleReaper {
					called = true
					return stub
				}).
				Build()
			require.NoError(t, err)

			require.Equal(t, tt.expectStart, called)
			require.Equal(t, tt.expectStart, stub.started)

			app.Cleanup()
			require.Equal(t, tt.expectStop, stub.stopped)
		})
	}
}
