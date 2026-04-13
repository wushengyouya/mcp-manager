package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/stretchr/testify/require"
)

type fakeRemoteRuntimeClient struct {
	connectStatus    mcpclient.RuntimeStatus
	connectErr       error
	disconnectStatus mcpclient.RuntimeStatus
	disconnectErr    error
	status           mcpclient.RuntimeStatus
	statusOK         bool
	statusErr        error
	tools            []mcp.Tool
	listToolsStatus  mcpclient.RuntimeStatus
	listToolsErr     error
	toolResult       *mcp.CallToolResult
	callToolStatus   mcpclient.RuntimeStatus
	callToolErr      error

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

func (f *fakeRemoteRuntimeClient) DisconnectWithEvidence(_ context.Context, serviceID string) (mcpclient.RuntimeStatus, error) {
	f.lastDisconnectedID = serviceID
	if f.disconnectStatus.ServiceID == "" {
		f.disconnectStatus.ServiceID = serviceID
	}
	return f.disconnectStatus, f.disconnectErr
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
			ServiceID:      "svc-remote",
			Status:         entity.ServiceStatusConnected,
			TransportType:  string(entity.TransportTypeStreamableHTTP),
			ExecutorID:     "executor@test@127.0.0.1:18081",
			SnapshotWriter: "executor@test@127.0.0.1:18081",
			RequestID:      "rpc-1",
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
	require.Equal(t, "executor@test@127.0.0.1:18081", snapshot.ExecutorID)
	require.Equal(t, "executor@test@127.0.0.1:18081", snapshot.SnapshotWriter)
	require.Equal(t, "rpc-1", snapshot.RequestID)
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
			ServiceID:      "svc-status",
			Status:         entity.ServiceStatusConnected,
			TransportType:  string(entity.TransportTypeStreamableHTTP),
			ExecutorID:     "executor@test@127.0.0.1:18081",
			SnapshotWriter: "executor@test@127.0.0.1:18081",
			RequestID:      "rpc-status-1",
			LastSeenAt:     &now,
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
	require.Equal(t, "executor@test@127.0.0.1:18081", snapshot.ExecutorID)
	require.Equal(t, "executor@test@127.0.0.1:18081", snapshot.SnapshotWriter)
	require.Equal(t, "rpc-status-1", snapshot.RequestID)
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
			ServiceID:      "svc-tools",
			Status:         entity.ServiceStatusConnected,
			TransportType:  string(entity.TransportTypeStreamableHTTP),
			ExecutorID:     "executor@test@127.0.0.1:18081",
			SnapshotWriter: "executor@test@127.0.0.1:18081",
			RequestID:      "rpc-list-1",
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
	require.Equal(t, "executor@test@127.0.0.1:18081", snapshot.ExecutorID)
	require.Equal(t, "rpc-list-1", snapshot.RequestID)
}

func TestRemoteRuntimeAdapterCallToolPersistsSnapshot(t *testing.T) {
	store := NewMemoryRuntimeStore()
	client := &fakeRemoteRuntimeClient{
		toolResult: &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}},
		callToolStatus: mcpclient.RuntimeStatus{
			ServiceID:      "svc-invoke",
			Status:         entity.ServiceStatusConnected,
			TransportType:  string(entity.TransportTypeStreamableHTTP),
			ExecutorID:     "executor@test@127.0.0.1:18081",
			SnapshotWriter: "executor@test@127.0.0.1:18081",
			RequestID:      "rpc-invoke-1",
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
	require.Equal(t, "executor@test@127.0.0.1:18081", snapshot.ExecutorID)
	require.Equal(t, "rpc-invoke-1", snapshot.RequestID)
}

func TestRemoteRuntimeAdapterDetectsOwnerConflictOnStatusWrite(t *testing.T) {
	store := NewMemoryRuntimeStore()
	now := time.Now()
	require.NoError(t, store.SaveSnapshot(context.Background(), mcpclient.RuntimeSnapshot{
		RuntimeStatus: mcpclient.RuntimeStatus{
			ServiceID:      "svc-conflict",
			Status:         entity.ServiceStatusConnected,
			ExecutorID:     "executor-a",
			SnapshotWriter: "executor-a",
			RequestID:      "rpc-old",
		},
		ObservedAt: now,
	}))

	client := &fakeRemoteRuntimeClient{
		status: mcpclient.RuntimeStatus{
			ServiceID:      "svc-conflict",
			Status:         entity.ServiceStatusConnected,
			ExecutorID:     "executor-b",
			SnapshotWriter: "executor-b",
			RequestID:      "rpc-new",
		},
		statusOK: true,
	}

	var events []OwnerConflictEvent
	adapter := NewRemoteRuntimeAdapter(
		client,
		WithRemoteRuntimeSnapshotStore(store),
		WithRemoteRuntimeStatusTimeout(50*time.Millisecond),
		WithRemoteOwnerConflictDetection(config.RuntimeConfig{OwnerConflictDetectionEnabled: true}),
		WithRemoteOwnerConflictReporter(func(event OwnerConflictEvent) {
			events = append(events, event)
		}),
	)
	adapter.nowFn = func() time.Time { return now.Add(time.Second) }

	_, ok := adapter.GetStatus("svc-conflict")
	require.True(t, ok)
	require.Len(t, events, 1)
	require.True(t, events[0].OwnerConflictDetected)
	require.Equal(t, "svc-conflict", events[0].ServiceID)
	require.Equal(t, "executor-a", events[0].OldExecutorID)
	require.Equal(t, "executor-b", events[0].NewExecutorID)
	require.Equal(t, "status", events[0].Operation)
	require.Equal(t, "rpc-new", events[0].RequestID)
	require.Equal(t, int64(defaultOwnerConflictDetectionWindow/time.Millisecond), events[0].WindowMS)
}

func TestRemoteRuntimeAdapterOwnerConflictDetectionDefaultOff(t *testing.T) {
	store := NewMemoryRuntimeStore()
	now := time.Now()
	require.NoError(t, store.SaveSnapshot(context.Background(), mcpclient.RuntimeSnapshot{
		RuntimeStatus: mcpclient.RuntimeStatus{
			ServiceID:      "svc-no-conflict",
			Status:         entity.ServiceStatusConnected,
			ExecutorID:     "executor-a",
			SnapshotWriter: "executor-a",
		},
		ObservedAt: now,
	}))

	client := &fakeRemoteRuntimeClient{
		status: mcpclient.RuntimeStatus{
			ServiceID:      "svc-no-conflict",
			Status:         entity.ServiceStatusConnected,
			ExecutorID:     "executor-b",
			SnapshotWriter: "executor-b",
			RequestID:      "rpc-new",
		},
		statusOK: true,
	}

	var events []OwnerConflictEvent
	adapter := NewRemoteRuntimeAdapter(
		client,
		WithRemoteRuntimeSnapshotStore(store),
		WithRemoteOwnerConflictReporter(func(event OwnerConflictEvent) {
			events = append(events, event)
		}),
	)

	_, ok := adapter.GetStatus("svc-no-conflict")
	require.True(t, ok)
	require.Empty(t, events)
}

func TestRemoteRuntimeAdapterDetectsOwnerConflictOnDisconnect(t *testing.T) {
	store := NewMemoryRuntimeStore()
	now := time.Now()
	require.NoError(t, store.SaveSnapshot(context.Background(), mcpclient.RuntimeSnapshot{
		RuntimeStatus: mcpclient.RuntimeStatus{
			ServiceID:      "svc-disconnect",
			Status:         entity.ServiceStatusConnected,
			ExecutorID:     "executor-a",
			SnapshotWriter: "executor-a",
			RequestID:      "rpc-old",
		},
		ObservedAt: now,
	}))

	client := &fakeRemoteRuntimeClient{
		disconnectStatus: mcpclient.RuntimeStatus{
			ServiceID:      "svc-disconnect",
			ExecutorID:     "executor-b",
			SnapshotWriter: "executor-b",
			RequestID:      "rpc-disconnect",
		},
	}

	var events []OwnerConflictEvent
	adapter := NewRemoteRuntimeAdapter(
		client,
		WithRemoteRuntimeSnapshotStore(store),
		WithRemoteOwnerConflictDetection(config.RuntimeConfig{OwnerConflictDetectionEnabled: true}),
		WithRemoteOwnerConflictReporter(func(event OwnerConflictEvent) {
			events = append(events, event)
		}),
	)
	adapter.nowFn = func() time.Time { return now.Add(time.Second) }

	require.NoError(t, adapter.Disconnect(context.Background(), "svc-disconnect"))
	require.Len(t, events, 1)
	require.Equal(t, "disconnect", events[0].Operation)
	require.Equal(t, "executor-a", events[0].OldExecutorID)
	require.Equal(t, "executor-b", events[0].NewExecutorID)

	_, ok, err := store.GetSnapshot(context.Background(), "svc-disconnect")
	require.NoError(t, err)
	require.False(t, ok)
}
