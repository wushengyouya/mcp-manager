package mcpclient

import (
	"testing"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

func TestValidateSessionModeAcceptsAutoWithoutSession(t *testing.T) {
	err := validateSessionMode(entity.TransportTypeStreamableHTTP, entity.TransportTypeStreamableHTTP, "auto", false)
	require.NoError(t, err)
}

func TestValidateSessionModeRejectsRequiredWithoutSession(t *testing.T) {
	err := validateSessionMode(entity.TransportTypeStreamableHTTP, entity.TransportTypeStreamableHTTP, "required", false)
	require.ErrorIs(t, err, ErrSessionRequired)
}

func TestValidateSessionModeRejectsDisabledWithSession(t *testing.T) {
	err := validateSessionMode(entity.TransportTypeStreamableHTTP, entity.TransportTypeStreamableHTTP, "disabled", true)
	require.ErrorIs(t, err, ErrSessionDisabled)
}

func TestValidateSessionModeRejectsRequiredWhenFallbackToSSE(t *testing.T) {
	err := validateSessionMode(entity.TransportTypeStreamableHTTP, entity.TransportTypeSSE, "required", false)
	require.ErrorIs(t, err, ErrSessionRequired)
}

func TestManagerHandleClientErrorRemovesClientOnSessionReconnectRequired(t *testing.T) {
	manager := NewManager(config.AppConfig{})
	client := &managedClient{
		service: &entity.MCPService{},
		runtime: RuntimeStatus{
			ServiceID:       "svc-ping",
			Status:          entity.ServiceStatusConnected,
			SessionIDExists: true,
		},
	}
	manager.items["svc-ping"] = client

	status, err := manager.handleClientError("svc-ping", client, ErrSessionReinitializeRequired)
	require.ErrorIs(t, err, ErrSessionReinitializeRequired)
	require.Equal(t, entity.ServiceStatusError, status.Status)
	require.False(t, status.SessionIDExists)

	_, ok := manager.GetStatus("svc-ping")
	require.False(t, ok)
}
