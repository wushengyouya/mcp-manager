package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/stretchr/testify/require"
)

type fakeRemoteRuntimeClient struct {
	connectStatus   mcpclient.RuntimeStatus
	connectErr      error
	disconnectErr   error
	status          mcpclient.RuntimeStatus
	statusOK        bool
	statusErr       error
	tools           []mcp.Tool
	listToolsStatus mcpclient.RuntimeStatus
	listToolsErr    error
	toolResult      *mcp.CallToolResult
	callToolStatus  mcpclient.RuntimeStatus
	callToolErr     error

	lastConnectedServiceID string
	lastDisconnectedID     string
	lastStatusID           string
	lastListToolsID        string
	lastCallToolServiceID  string
	lastCallToolName       string
	lastCallToolArgs       map[string]any
}

func (f *fakeRemoteRuntimeClient) Connect(_ context.Context, service *entity.MCPService) (mcpclient.RuntimeStatus, error) {
	if service != nil {
		f.lastConnectedServiceID = service.ID
	}
	return f.connectStatus, f.connectErr
}

func (f *fakeRemoteRuntimeClient) Disconnect(_ context.Context, serviceID string) error {
	f.lastDisconnectedID = serviceID
	return f.disconnectErr
}

func (f *fakeRemoteRuntimeClient) GetStatus(_ context.Context, serviceID string) (mcpclient.RuntimeStatus, bool, error) {
	f.lastStatusID = serviceID
	return f.status, f.statusOK, f.statusErr
}

func (f *fakeRemoteRuntimeClient) ListTools(_ context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error) {
	f.lastListToolsID = serviceID
	return f.tools, f.listToolsStatus, f.listToolsErr
}

func (f *fakeRemoteRuntimeClient) CallTool(_ context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	f.lastCallToolServiceID = serviceID
	f.lastCallToolName = name
	f.lastCallToolArgs = args
	return f.toolResult, f.callToolStatus, f.callToolErr
}

func TestRemoteRuntimeAdapterConnectPersistsSnapshot(t *testing.T) {
	store := NewMemoryRuntimeStore()
	client := &fakeRemoteRuntimeClient{
		connectStatus: mcpclient.RuntimeStatus{
			ServiceID:     "svc-remote",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
		},
	}
	adapter := NewRemoteRuntimeAdapter(client, WithRemoteRuntimeSnapshotStore(store))

	status, err := adapter.Connect(context.Background(), &entity.MCPService{Base: entity.Base{ID: "svc-remote"}})
	require.NoError(t, err)
	require.Equal(t, "svc-remote", client.lastConnectedServiceID)
	require.Equal(t, entity.ServiceStatusConnected, status.Status)

	snapshot, ok, err := store.GetSnapshot(context.Background(), "svc-remote")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.ServiceStatusConnected, snapshot.Status)
}

func TestRemoteRuntimeAdapterDisconnectDeletesSnapshot(t *testing.T) {
	store := NewMemoryRuntimeStore()
	require.NoError(t, store.SaveSnapshot(context.Background(), mcpclient.RuntimeSnapshot{
		RuntimeStatus: mcpclient.RuntimeStatus{ServiceID: "svc-remote", Status: entity.ServiceStatusConnected},
		ObservedAt:    time.Now(),
	}))

	client := &fakeRemoteRuntimeClient{}
	adapter := NewRemoteRuntimeAdapter(client, WithRemoteRuntimeSnapshotStore(store))

	require.NoError(t, adapter.Disconnect(context.Background(), "svc-remote"))
	require.Equal(t, "svc-remote", client.lastDisconnectedID)

	_, ok, err := store.GetSnapshot(context.Background(), "svc-remote")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestRemoteRuntimeAdapterGetStatusPersistsSnapshot(t *testing.T) {
	store := NewMemoryRuntimeStore()
	now := time.Now()
	client := &fakeRemoteRuntimeClient{
		status: mcpclient.RuntimeStatus{
			ServiceID:     "svc-status",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
			LastSeenAt:    &now,
		},
		statusOK: true,
	}
	adapter := NewRemoteRuntimeAdapter(
		client,
		WithRemoteRuntimeSnapshotStore(store),
		WithRemoteRuntimeStatusTimeout(50*time.Millisecond),
	)

	status, ok := adapter.GetStatus("svc-status")
	require.True(t, ok)
	require.Equal(t, "svc-status", client.lastStatusID)
	require.Equal(t, entity.ServiceStatusConnected, status.Status)

	snapshot, exists, err := store.GetSnapshot(context.Background(), "svc-status")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, entity.ServiceStatusConnected, snapshot.Status)
}

func TestRemoteRuntimeAdapterGetStatusErrorFallsBackToMissing(t *testing.T) {
	client := &fakeRemoteRuntimeClient{statusErr: errors.New("rpc unavailable")}
	adapter := NewRemoteRuntimeAdapter(client)

	status, ok := adapter.GetStatus("svc-status")
	require.False(t, ok)
	require.Equal(t, mcpclient.RuntimeStatus{}, status)
}

func TestRemoteRuntimeAdapterListToolsPersistsSnapshot(t *testing.T) {
	store := NewMemoryRuntimeStore()
	client := &fakeRemoteRuntimeClient{
		tools: []mcp.Tool{{Name: "search"}},
		listToolsStatus: mcpclient.RuntimeStatus{
			ServiceID:     "svc-tools",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
		},
	}
	adapter := NewRemoteRuntimeAdapter(client, WithRemoteRuntimeSnapshotStore(store))

	tools, status, err := adapter.ListTools(context.Background(), "svc-tools")
	require.NoError(t, err)
	require.Equal(t, "svc-tools", client.lastListToolsID)
	require.Len(t, tools, 1)
	require.Equal(t, entity.ServiceStatusConnected, status.Status)

	snapshot, ok, err := store.GetSnapshot(context.Background(), "svc-tools")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.ServiceStatusConnected, snapshot.Status)
}

func TestRemoteRuntimeAdapterCallToolPersistsSnapshot(t *testing.T) {
	store := NewMemoryRuntimeStore()
	client := &fakeRemoteRuntimeClient{
		toolResult: &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}},
		callToolStatus: mcpclient.RuntimeStatus{
			ServiceID:     "svc-invoke",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
		},
	}
	adapter := NewRemoteRuntimeAdapter(client, WithRemoteRuntimeSnapshotStore(store))

	result, status, err := adapter.CallTool(context.Background(), "svc-invoke", "echo", map[string]any{"text": "hello"})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "svc-invoke", client.lastCallToolServiceID)
	require.Equal(t, "echo", client.lastCallToolName)
	require.Equal(t, map[string]any{"text": "hello"}, client.lastCallToolArgs)
	require.Equal(t, entity.ServiceStatusConnected, status.Status)

	snapshot, ok, err := store.GetSnapshot(context.Background(), "svc-invoke")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.ServiceStatusConnected, snapshot.Status)
}
