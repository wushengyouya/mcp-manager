package mcpclient

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	transport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

type fakeRuntimeClient struct {
	sessionID string

	startErr      error
	initializeErr error
	listToolsErr  error
	callToolErr   error
	pingErr       error
	closeErr      error
	closeHook     func() error

	initializeResult *mcp.InitializeResult
	listToolsResult  *mcp.ListToolsResult
	callToolResult   *mcp.CallToolResult

	startCalls      int
	initializeCalls int
	closeCalls      int

	lastCallToolRequest mcp.CallToolRequest
	lastListToolsReq    mcp.ListToolsRequest

	onNotification   func(mcp.JSONRPCNotification)
	onConnectionLost func(error)
}

func (f *fakeRuntimeClient) Start(context.Context) error {
	f.startCalls++
	return f.startErr
}

func (f *fakeRuntimeClient) Initialize(context.Context, mcp.InitializeRequest) (*mcp.InitializeResult, error) {
	f.initializeCalls++
	if f.initializeErr != nil {
		return nil, f.initializeErr
	}
	return f.initializeResult, nil
}

func (f *fakeRuntimeClient) Close() error {
	f.closeCalls++
	if f.closeHook != nil {
		if err := f.closeHook(); err != nil {
			return err
		}
	}
	return f.closeErr
}

func (f *fakeRuntimeClient) ListTools(context.Context, mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	if f.listToolsErr != nil {
		return nil, f.listToolsErr
	}
	return f.listToolsResult, nil
}

func (f *fakeRuntimeClient) CallTool(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	f.lastCallToolRequest = req
	if f.callToolErr != nil {
		return nil, f.callToolErr
	}
	return f.callToolResult, nil
}

func (f *fakeRuntimeClient) Ping(context.Context) error {
	return f.pingErr
}

func (f *fakeRuntimeClient) GetSessionId() string {
	return f.sessionID
}

func (f *fakeRuntimeClient) OnNotification(handler func(notification mcp.JSONRPCNotification)) {
	f.onNotification = handler
}

func (f *fakeRuntimeClient) OnConnectionLost(handler func(error)) {
	f.onConnectionLost = handler
}

func withClientFactories(t *testing.T, setup func()) {
	t.Helper()
	oldStdio := newStdioClient
	oldSSE := newSSEClient
	oldHTTP := newStreamableHTTPClient
	t.Cleanup(func() {
		newStdioClient = oldStdio
		newSSEClient = oldSSE
		newStreamableHTTPClient = oldHTTP
	})
	setup()
}

func testInitializeResult() *mcp.InitializeResult {
	return &mcp.InitializeResult{
		ProtocolVersion: "2025-03-26",
		Capabilities: mcp.ServerCapabilities{
			Tools: &struct {
				ListChanged bool `json:"listChanged,omitempty"`
			}{ListChanged: true},
			Logging: &struct{}{},
			Prompts: &struct {
				ListChanged bool `json:"listChanged,omitempty"`
			}{},
			Roots:    &struct{}{},
			Sampling: &struct{}{},
		},
	}
}

func TestBuildHeadersIncludesDefaultsAndBearer(t *testing.T) {
	headers := buildHeaders(&entity.MCPService{
		BearerToken:   "secret",
		CustomHeaders: entity.JSONStringMap{"X-Test": "1"},
	})
	require.Equal(t, "mcp-manager", headers["User-Agent"])
	require.Equal(t, "1", headers["X-Test"])
	require.Equal(t, "Bearer secret", headers["Authorization"])
}

func TestBuildClientDispatchesByTransport(t *testing.T) {
	stdioClient := &fakeRuntimeClient{}
	sseClient := &fakeRuntimeClient{}
	httpClient := &fakeRuntimeClient{}
	stdioCalled := false
	sseCalled := false
	httpCalled := false

	withClientFactories(t, func() {
		newStdioClient = func(command string, env []string, args []string) (runtimeClient, error) {
			stdioCalled = true
			require.Equal(t, "echo", command)
			require.Equal(t, []string{"A=1"}, env)
			require.Equal(t, []string{"hello"}, args)
			return stdioClient, nil
		}
		newSSEClient = func(url string, headers map[string]string, httpClientArg *http.Client) (runtimeClient, error) {
			sseCalled = true
			require.Equal(t, "http://sse.test/events", url)
			require.Equal(t, "Bearer token", headers["Authorization"])
			require.NotNil(t, httpClientArg)
			return sseClient, nil
		}
		newStreamableHTTPClient = func(url string, _ ...transport.StreamableHTTPCOption) (runtimeClient, error) {
			httpCalled = true
			require.Equal(t, "http://http.test/mcp", url)
			return httpClient, nil
		}
	})

	client, actualTransport, err := buildClient(&entity.MCPService{
		TransportType: entity.TransportTypeStdio,
		Command:       "echo",
		Args:          entity.JSONStringList{"hello"},
		Env:           entity.JSONStringMap{"A": "1"},
	})
	require.NoError(t, err)
	require.Same(t, stdioClient, client)
	require.Equal(t, entity.TransportTypeStdio, actualTransport)

	client, actualTransport, err = buildClient(&entity.MCPService{
		TransportType: entity.TransportTypeSSE,
		URL:           "http://sse.test/events",
		Timeout:       5,
		BearerToken:   "token",
	})
	require.NoError(t, err)
	require.Same(t, sseClient, client)
	require.Equal(t, entity.TransportTypeSSE, actualTransport)

	client, actualTransport, err = buildClient(&entity.MCPService{
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://http.test/mcp",
		Timeout:       5,
		ListenEnabled: true,
	})
	require.NoError(t, err)
	require.Same(t, httpClient, client)
	require.Equal(t, entity.TransportTypeStreamableHTTP, actualTransport)

	require.True(t, stdioCalled)
	require.True(t, sseCalled)
	require.True(t, httpCalled)
}

func TestBuildClientRejectsUnsupportedTransport(t *testing.T) {
	client, transportType, err := buildClient(&entity.MCPService{TransportType: "invalid"})
	require.Nil(t, client)
	require.Empty(t, transportType)
	require.ErrorContains(t, err, "unsupported transport")
}

func TestManagedClientInitializeSuccess(t *testing.T) {
	fake := &fakeRuntimeClient{
		sessionID:        "sess-1",
		initializeResult: testInitializeResult(),
		listToolsResult:  &mcp.ListToolsResult{},
		callToolResult:   &mcp.CallToolResult{},
	}
	item := &managedClient{
		service: &entity.MCPService{
			Base:          entity.Base{ID: "svc-1"},
			TransportType: entity.TransportTypeStreamableHTTP,
			ListenEnabled: true,
			Timeout:       1,
			SessionMode:   "auto",
			CompatMode:    "off",
			FailureCount:  2,
			LastError:     "old",
		},
		client:          fake,
		actualTransport: entity.TransportTypeStreamableHTTP,
		runtime: RuntimeStatus{
			ServiceID:             "svc-1",
			Status:                entity.ServiceStatusConnecting,
			TransportType:         string(entity.TransportTypeStreamableHTTP),
			ListenEnabled:         true,
			TransportCapabilities: map[string]any{},
		},
	}

	require.NoError(t, item.initialize(config.AppConfig{Name: "app", Version: "1.0"}))
	require.Equal(t, 1, fake.startCalls)
	require.Equal(t, 1, fake.initializeCalls)

	status := item.runtimeStatus()
	require.Equal(t, entity.ServiceStatusConnected, status.Status)
	require.Equal(t, "2025-03-26", status.ProtocolVersion)
	require.True(t, status.SessionIDExists)
	require.True(t, status.ListenActive)
	require.True(t, status.TransportCapabilities["tools"].(bool))
	require.NotNil(t, status.LastSeenAt)
}

func TestManagedClientInitializeFallbackToLegacySSE(t *testing.T) {
	primary := &fakeRuntimeClient{initializeErr: errors.New("primary initialize failed")}
	fallback := &fakeRuntimeClient{initializeResult: testInitializeResult()}

	withClientFactories(t, func() {
		newSSEClient = func(url string, headers map[string]string, httpClientArg *http.Client) (runtimeClient, error) {
			require.Equal(t, "http://fallback.test/mcp", url)
			require.NotNil(t, headers)
			require.NotNil(t, httpClientArg)
			return fallback, nil
		}
	})

	item := &managedClient{
		service: &entity.MCPService{
			Base:          entity.Base{ID: "svc-fallback"},
			URL:           "http://fallback.test/mcp",
			TransportType: entity.TransportTypeStreamableHTTP,
			CompatMode:    "allow_legacy_sse",
			ListenEnabled: true,
			Timeout:       1,
		},
		client:          primary,
		actualTransport: entity.TransportTypeStreamableHTTP,
		runtime: RuntimeStatus{
			ServiceID:             "svc-fallback",
			Status:                entity.ServiceStatusConnecting,
			TransportType:         string(entity.TransportTypeStreamableHTTP),
			TransportCapabilities: map[string]any{},
		},
	}
	item.bindRuntimeCallbacks(primary)

	require.NoError(t, item.initialize(config.AppConfig{Name: "app", Version: "1.0"}))
	require.Equal(t, 1, primary.startCalls)
	require.Equal(t, 1, fallback.startCalls)
	require.Equal(t, 1, primary.closeCalls)
	require.Equal(t, entity.TransportTypeSSE, item.actualTransport)
	require.Equal(t, entity.ServiceStatusConnected, item.runtimeStatus().Status)
	require.NotNil(t, fallback.onNotification)
	require.NotNil(t, fallback.onConnectionLost)

	primary.onConnectionLost(errors.New("stale primary lost"))
	status := item.runtimeStatus()
	require.Equal(t, entity.ServiceStatusConnected, status.Status)
	require.Empty(t, status.LastError)

	before := time.Now()
	fallback.onNotification(mcp.JSONRPCNotification{})
	status = item.runtimeStatus()
	require.NotNil(t, status.LastSeenAt)
	require.True(t, status.LastSeenAt.After(before) || status.LastSeenAt.Equal(before))
	require.True(t, status.ListenActive)

	fallback.onConnectionLost(errors.New("fallback lost"))
	status = item.runtimeStatus()
	require.Equal(t, entity.ServiceStatusError, status.Status)
	require.Equal(t, "fallback lost", status.LastError)
	require.Equal(t, "fallback lost", status.ListenLastError)
	require.False(t, status.ListenActive)

	require.NoError(t, item.close())
	require.Equal(t, 1, fallback.closeCalls)
	require.Equal(t, 1, primary.closeCalls)
}

func TestManagedClientInitializeFallbackSwitchKeepsClientAccessSynchronized(t *testing.T) {
	primary := &fakeRuntimeClient{initializeErr: errors.New("primary initialize failed")}
	fallback := &fakeRuntimeClient{initializeResult: testInitializeResult()}
	closeStarted := make(chan struct{})
	releaseClose := make(chan struct{})
	stopReads := make(chan struct{})
	readDone := make(chan struct{})

	primary.closeHook = func() error {
		close(closeStarted)
		<-releaseClose
		return nil
	}

	withClientFactories(t, func() {
		newSSEClient = func(url string, headers map[string]string, httpClientArg *http.Client) (runtimeClient, error) {
			return fallback, nil
		}
	})

	item := &managedClient{
		service: &entity.MCPService{
			Base:          entity.Base{ID: "svc-fallback-race"},
			URL:           "http://fallback.test/mcp",
			TransportType: entity.TransportTypeStreamableHTTP,
			CompatMode:    "allow_legacy_sse",
			ListenEnabled: true,
			Timeout:       1,
		},
		client:          primary,
		actualTransport: entity.TransportTypeStreamableHTTP,
		runtime: RuntimeStatus{
			ServiceID:             "svc-fallback-race",
			Status:                entity.ServiceStatusConnecting,
			TransportType:         string(entity.TransportTypeStreamableHTTP),
			TransportCapabilities: map[string]any{},
		},
	}
	item.bindRuntimeCallbacks(primary)

	errCh := make(chan error, 1)
	go func() {
		errCh <- item.initialize(config.AppConfig{Name: "app", Version: "1.0"})
	}()

	<-closeStarted
	go func() {
		defer close(readDone)
		for {
			select {
			case <-stopReads:
				return
			default:
				_ = item.runtimeStatus()
			}
		}
	}()

	close(releaseClose)
	require.NoError(t, <-errCh)
	close(stopReads)
	<-readDone

	status := item.runtimeStatus()
	require.Equal(t, entity.ServiceStatusConnected, status.Status)
	require.Equal(t, entity.TransportTypeSSE, item.actualTransport)
	require.Equal(t, 1, primary.closeCalls)
	require.Equal(t, 1, fallback.startCalls)
}

func TestManagedClientInitializeSessionModeFailureMarksTerminalError(t *testing.T) {
	fake := &fakeRuntimeClient{initializeResult: testInitializeResult()}
	item := &managedClient{
		service: &entity.MCPService{
			Base:          entity.Base{ID: "svc-required"},
			TransportType: entity.TransportTypeStreamableHTTP,
			SessionMode:   "required",
			Timeout:       1,
		},
		client:          fake,
		actualTransport: entity.TransportTypeStreamableHTTP,
		runtime: RuntimeStatus{
			ServiceID:             "svc-required",
			Status:                entity.ServiceStatusConnecting,
			TransportType:         string(entity.TransportTypeStreamableHTTP),
			TransportCapabilities: map[string]any{},
		},
	}

	err := item.initialize(config.AppConfig{Name: "app", Version: "1.0"})
	require.ErrorIs(t, err, ErrSessionRequired)
	status := item.runtimeStatus()
	require.Equal(t, entity.ServiceStatusError, status.Status)
	require.Contains(t, status.LastError, ErrSessionRequired.Error())
}

func TestManagedClientStateTransitions(t *testing.T) {
	fake := &fakeRuntimeClient{sessionID: "sess-1"}
	item := &managedClient{
		service: &entity.MCPService{ListenEnabled: true},
		client:  fake,
		runtime: RuntimeStatus{
			Status:       entity.ServiceStatusConnected,
			ListenActive: true,
			FailureCount: 2,
		},
	}

	item.markError(errors.New("temporary failure"))
	status := item.runtimeStatus()
	require.Equal(t, "temporary failure", status.LastError)
	require.False(t, status.ListenActive)

	item.markSeen()
	status = item.runtimeStatus()
	require.Equal(t, entity.ServiceStatusConnected, status.Status)
	require.Zero(t, status.FailureCount)
	require.Empty(t, status.LastError)
	require.True(t, status.ListenActive)
	require.NotNil(t, status.LastSeenAt)
	require.True(t, status.SessionIDExists)

	item.applyHealthState(entity.ServiceStatusError, 3, "boom")
	status = item.runtimeStatus()
	require.Equal(t, entity.ServiceStatusError, status.Status)
	require.Equal(t, 3, status.FailureCount)
	require.Equal(t, "boom", status.ListenLastError)
	require.False(t, status.ListenActive)

	item.applyHealthState(entity.ServiceStatusConnected, 0, "")
	status = item.runtimeStatus()
	require.Equal(t, entity.ServiceStatusConnected, status.Status)
	require.True(t, status.ListenActive)
	require.Empty(t, status.ListenLastError)

	require.NoError(t, item.close())
	require.NoError(t, item.close())
	require.Equal(t, 1, fake.closeCalls)
}

func TestManagerConnectDisconnectAndIDs(t *testing.T) {
	connectClient := &fakeRuntimeClient{sessionID: "sess-1", initializeResult: testInitializeResult()}
	withClientFactories(t, func() {
		newStreamableHTTPClient = func(url string, _ ...transport.StreamableHTTPCOption) (runtimeClient, error) {
			require.Equal(t, "http://svc.test/mcp", url)
			return connectClient, nil
		}
	})

	manager := NewManager(config.AppConfig{Name: "app", Version: "1.0"})
	serviceItem := &entity.MCPService{
		Base:          entity.Base{ID: "svc-connect"},
		Name:          "svc-connect",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc.test/mcp",
		Timeout:       1,
	}

	status, err := manager.Connect(context.Background(), serviceItem)
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusConnected, status.Status)
	require.ElementsMatch(t, []string{"svc-connect"}, manager.IDs())

	gotStatus, ok := manager.GetStatus("svc-connect")
	require.True(t, ok)
	require.Equal(t, entity.ServiceStatusConnected, gotStatus.Status)

	require.NoError(t, manager.Disconnect(context.Background(), "svc-connect"))
	require.ErrorIs(t, manager.Disconnect(context.Background(), "svc-connect"), ErrServiceNotConnected)
}

func TestManagerListToolsCallToolAndPing(t *testing.T) {
	now := time.Now()
	fake := &fakeRuntimeClient{
		sessionID:        "sess-1",
		listToolsResult:  &mcp.ListToolsResult{Tools: []mcp.Tool{{Name: "search"}}},
		callToolResult:   &mcp.CallToolResult{IsError: false},
		initializeResult: testInitializeResult(),
	}
	manager := NewManager(config.AppConfig{})
	manager.items["svc-ops"] = &managedClient{
		service: &entity.MCPService{Base: entity.Base{ID: "svc-ops"}, ListenEnabled: true},
		client:  fake,
		runtime: RuntimeStatus{
			ServiceID:       "svc-ops",
			Status:          entity.ServiceStatusConnected,
			LastSeenAt:      &now,
			SessionIDExists: true,
		},
	}

	tools, status, err := manager.ListTools(context.Background(), "svc-ops")
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, entity.ServiceStatusConnected, status.Status)
	require.NotNil(t, status.LastUsedAt)
	require.Zero(t, status.InFlight)
	firstUsedAt := *status.LastUsedAt

	result, status, err := manager.CallTool(context.Background(), "svc-ops", "search", map[string]any{"q": "hello"})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, "search", fake.lastCallToolRequest.Params.Name)
	require.NotNil(t, status.LastUsedAt)
	require.True(t, status.LastUsedAt.After(firstUsedAt) || status.LastUsedAt.Equal(firstUsedAt))
	callUsedAt := *status.LastUsedAt

	status, err = manager.Ping(context.Background(), "svc-ops")
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusConnected, status.Status)
	require.NotNil(t, status.LastUsedAt)
	require.Equal(t, callUsedAt, *status.LastUsedAt)
}

func TestManagerPingDoesNotUpdateLastUsedAt(t *testing.T) {
	lastUsedAt := time.Now().Add(-time.Minute)
	fake := &fakeRuntimeClient{sessionID: "sess-1"}
	manager := NewManager(config.AppConfig{})
	manager.items["svc-ping"] = &managedClient{
		service: &entity.MCPService{Base: entity.Base{ID: "svc-ping"}, ListenEnabled: true},
		client:  fake,
		runtime: RuntimeStatus{
			ServiceID:       "svc-ping",
			Status:          entity.ServiceStatusConnected,
			LastUsedAt:      &lastUsedAt,
			SessionIDExists: true,
		},
	}

	status, err := manager.Ping(context.Background(), "svc-ping")
	require.NoError(t, err)
	require.NotNil(t, status.LastUsedAt)
	require.Equal(t, lastUsedAt, *status.LastUsedAt)
}

func TestManagerOperationsHandleErrors(t *testing.T) {
	plainErr := errors.New("plain failure")
	fake := &fakeRuntimeClient{
		sessionID:        "sess-1",
		listToolsErr:     plainErr,
		callToolErr:      plainErr,
		pingErr:          transport.ErrSessionTerminated,
		initializeResult: testInitializeResult(),
	}
	manager := NewManager(config.AppConfig{})
	manager.items["svc-errors"] = &managedClient{
		service: &entity.MCPService{Base: entity.Base{ID: "svc-errors"}},
		client:  fake,
		runtime: RuntimeStatus{ServiceID: "svc-errors", Status: entity.ServiceStatusConnected, SessionIDExists: true},
	}

	_, status, err := manager.ListTools(context.Background(), "svc-errors")
	require.ErrorIs(t, err, plainErr)
	require.Equal(t, "plain failure", status.LastError)
	require.Zero(t, status.InFlight)
	require.NotNil(t, status.LastUsedAt)

	_, _, err = manager.CallTool(context.Background(), "svc-errors", "search", nil)
	require.ErrorIs(t, err, plainErr)

	_, err = manager.Ping(context.Background(), "svc-errors")
	require.ErrorIs(t, err, ErrSessionReinitializeRequired)
	_, ok := manager.GetStatus("svc-errors")
	require.False(t, ok)
}

func TestMarshalResult(t *testing.T) {
	require.Nil(t, MarshalResult(nil))

	result := MarshalResult(&mcp.CallToolResult{
		IsError:           true,
		StructuredContent: map[string]any{"ok": true},
		Content:           []mcp.Content{mcp.NewTextContent("hello")},
	})
	require.Equal(t, true, result["is_error"])
	require.Equal(t, map[string]any{"ok": true}, result["structured_content"])
	content := result["content"].([]any)
	require.Len(t, content, 1)
}
