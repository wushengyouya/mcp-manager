package rpc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/stretchr/testify/require"
)

type fakeExecutor struct {
	connectStatus   mcpclient.RuntimeStatus
	connectErr      error
	disconnectErr   error
	status          mcpclient.RuntimeStatus
	statusFound     bool
	statusErr       error
	tools           []mcp.Tool
	listToolsStatus mcpclient.RuntimeStatus
	listToolsErr    error
	toolResult      *mcp.CallToolResult
	callToolStatus  mcpclient.RuntimeStatus
	callToolErr     error
	pingErr         error

	lastConnectedService  *entity.MCPService
	lastDisconnectedID    string
	lastStatusID          string
	lastListToolsID       string
	lastCallToolServiceID string
	lastCallToolName      string
	lastCallToolArguments map[string]any
}

func (f *fakeExecutor) Connect(_ context.Context, service *entity.MCPService) (mcpclient.RuntimeStatus, error) {
	f.lastConnectedService = service
	return f.connectStatus, f.connectErr
}

func (f *fakeExecutor) Disconnect(_ context.Context, serviceID string) error {
	f.lastDisconnectedID = serviceID
	return f.disconnectErr
}

func (f *fakeExecutor) GetStatus(_ context.Context, serviceID string) (mcpclient.RuntimeStatus, bool, error) {
	f.lastStatusID = serviceID
	return f.status, f.statusFound, f.statusErr
}

func (f *fakeExecutor) ListTools(_ context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error) {
	f.lastListToolsID = serviceID
	return f.tools, f.listToolsStatus, f.listToolsErr
}

func (f *fakeExecutor) CallTool(_ context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	f.lastCallToolServiceID = serviceID
	f.lastCallToolName = name
	f.lastCallToolArguments = args
	return f.toolResult, f.callToolStatus, f.callToolErr
}

func (f *fakeExecutor) Ping(context.Context) error {
	return f.pingErr
}

func TestClientRoundTrip(t *testing.T) {
	const executorID = "executor@test@127.0.0.1:18081"
	executor := &fakeExecutor{
		connectStatus: mcpclient.RuntimeStatus{
			ServiceID:     "svc-rpc",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
		},
		status: mcpclient.RuntimeStatus{
			ServiceID:     "svc-rpc",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
		},
		statusFound: true,
		tools:       []mcp.Tool{{Name: "search"}},
		listToolsStatus: mcpclient.RuntimeStatus{
			ServiceID:     "svc-rpc",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
		},
		toolResult: &mcp.CallToolResult{
			StructuredContent: map[string]any{"echo": "hello"},
			Content:           []mcp.Content{mcp.NewTextContent("hello")},
		},
		callToolStatus: mcpclient.RuntimeStatus{
			ServiceID:     "svc-rpc",
			Status:        entity.ServiceStatusConnected,
			TransportType: string(entity.TransportTypeStreamableHTTP),
		},
	}
	server := httptest.NewServer(NewHandler(executor, WithExecutorID(executorID)))
	defer server.Close()

	client := NewClient(server.URL, WithHTTPClient(server.Client()))
	serviceItem := &entity.MCPService{
		Base:          entity.Base{ID: "svc-rpc"},
		Name:          "svc-rpc",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-rpc.test/mcp",
	}

	connectStatus, err := client.Connect(context.Background(), serviceItem)
	require.NoError(t, err)
	require.Equal(t, "svc-rpc", executor.lastConnectedService.ID)
	require.Equal(t, entity.ServiceStatusConnected, connectStatus.Status)
	require.Equal(t, executorID, connectStatus.ExecutorID)
	require.Equal(t, executorID, connectStatus.SnapshotWriter)
	require.NotEmpty(t, connectStatus.RequestID)

	status, found, err := client.GetStatus(context.Background(), "svc-rpc")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "svc-rpc", executor.lastStatusID)
	require.Equal(t, entity.ServiceStatusConnected, status.Status)
	require.Equal(t, executorID, status.ExecutorID)
	require.NotEmpty(t, status.RequestID)

	tools, toolStatus, err := client.ListTools(context.Background(), "svc-rpc")
	require.NoError(t, err)
	require.Equal(t, "svc-rpc", executor.lastListToolsID)
	require.Len(t, tools, 1)
	require.Equal(t, entity.ServiceStatusConnected, toolStatus.Status)
	require.Equal(t, executorID, toolStatus.ExecutorID)
	require.NotEmpty(t, toolStatus.RequestID)

	result, invokeStatus, err := client.CallTool(context.Background(), "svc-rpc", "search", map[string]any{"q": "hello"})
	require.NoError(t, err)
	require.Equal(t, "svc-rpc", executor.lastCallToolServiceID)
	require.Equal(t, "search", executor.lastCallToolName)
	require.Equal(t, map[string]any{"q": "hello"}, executor.lastCallToolArguments)
	require.NotNil(t, result)
	require.Equal(t, entity.ServiceStatusConnected, invokeStatus.Status)
	require.Equal(t, executorID, invokeStatus.ExecutorID)
	require.NotEmpty(t, invokeStatus.RequestID)

	disconnectStatus, err := client.DisconnectWithEvidence(context.Background(), "svc-rpc")
	require.NoError(t, err)
	require.Equal(t, "svc-rpc", executor.lastDisconnectedID)
	require.Equal(t, "svc-rpc", disconnectStatus.ServiceID)
	require.Equal(t, executorID, disconnectStatus.ExecutorID)
	require.Equal(t, executorID, disconnectStatus.SnapshotWriter)
	require.NotEmpty(t, disconnectStatus.RequestID)
	require.NoError(t, client.PingExecutor(context.Background()))
}

func TestClientPropagatesExecutorErrors(t *testing.T) {
	executor := &fakeExecutor{
		connectStatus: mcpclient.RuntimeStatus{ServiceID: "svc-rpc", Status: entity.ServiceStatusError},
		connectErr:    errors.New("connect failed"),
		pingErr:       errors.New("ping failed"),
	}
	server := httptest.NewServer(NewHandler(executor, WithExecutorID("executor@test@127.0.0.1:18081")))
	defer server.Close()

	client := NewClient(server.URL, WithHTTPClient(server.Client()))

	status, err := client.Connect(context.Background(), &entity.MCPService{Base: entity.Base{ID: "svc-rpc"}})
	require.Error(t, err)
	require.Equal(t, entity.ServiceStatusError, status.Status)
	require.Contains(t, err.Error(), "connect failed")

	err = client.PingExecutor(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "ping failed")
}

func TestHandlerRejectsInvalidPayload(t *testing.T) {
	executor := &fakeExecutor{}
	server := httptest.NewServer(NewHandler(executor))
	defer server.Close()

	resp, err := server.Client().Post(server.URL+ConnectPath, "application/json", http.NoBody)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
