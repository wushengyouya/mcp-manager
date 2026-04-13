package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
)

const defaultHTTPTimeout = 5 * time.Second

// ClientOption 定义 RPC client 选项。
type ClientOption func(*Client)

// WithHTTPClient 注入自定义 HTTP client。
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// Client 定义最小内部 RPC client。
type Client struct {
	baseURL    string
	httpClient *http.Client
}

var rpcRequestCounter atomic.Uint64

// NewClient 创建内部 RPC client。
func NewClient(baseURL string, opts ...ClientOption) *Client {
	client := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

// Connect 调用远程连接接口。
func (c *Client) Connect(ctx context.Context, service *entity.MCPService) (mcpclient.RuntimeStatus, error) {
	req := ConnectServiceRequest{ServiceSnapshot: service, RequestID: newRequestID()}
	if service != nil {
		req.ServiceID = service.ID
	}
	var resp ConnectServiceResponse
	code, err := c.doJSON(ctx, http.MethodPost, ConnectPath, req, &resp)
	if err != nil {
		return mcpclient.RuntimeStatus{}, err
	}
	if err := endpointError(code, resp.Error); err != nil {
		return resp.Status, err
	}
	return resp.Status, nil
}

// Disconnect 调用远程断开接口。
func (c *Client) Disconnect(ctx context.Context, serviceID string) error {
	_, err := c.DisconnectWithEvidence(ctx, serviceID)
	return err
}

// DisconnectWithEvidence 调用远程断开接口并返回诊断信息。
func (c *Client) DisconnectWithEvidence(ctx context.Context, serviceID string) (mcpclient.RuntimeStatus, error) {
	var resp DisconnectServiceResponse
	code, err := c.doJSON(ctx, http.MethodPost, DisconnectPath, DisconnectServiceRequest{ServiceID: serviceID, RequestID: newRequestID()}, &resp)
	if err != nil {
		return mcpclient.RuntimeStatus{ServiceID: serviceID}, err
	}
	status := mcpclient.RuntimeStatus{
		ServiceID:      resp.ServiceID,
		ExecutorID:     resp.ExecutorID,
		SnapshotWriter: resp.ExecutorID,
		RequestID:      resp.RequestID,
	}
	if status.ServiceID == "" {
		status.ServiceID = serviceID
	}
	if err := endpointError(code, resp.Error); err != nil {
		return status, err
	}
	return status, nil
}

// GetStatus 调用远程状态接口。
func (c *Client) GetStatus(ctx context.Context, serviceID string) (mcpclient.RuntimeStatus, bool, error) {
	var resp GetRuntimeStatusResponse
	code, err := c.doJSON(ctx, http.MethodPost, StatusPath, GetRuntimeStatusRequest{ServiceID: serviceID, RequestID: newRequestID()}, &resp)
	if err != nil {
		return mcpclient.RuntimeStatus{}, false, err
	}
	if err := endpointError(code, resp.Error); err != nil {
		return resp.Status, resp.Found, err
	}
	return resp.Status, resp.Found, nil
}

// ListTools 调用远程工具目录接口。
func (c *Client) ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error) {
	var resp ListToolsResponse
	code, err := c.doJSON(ctx, http.MethodPost, ListToolsPath, ListToolsRequest{ServiceID: serviceID, RequestID: newRequestID()}, &resp)
	if err != nil {
		return nil, mcpclient.RuntimeStatus{}, err
	}
	if err := endpointError(code, resp.Error); err != nil {
		return resp.Tools, resp.Status, err
	}
	return resp.Tools, resp.Status, nil
}

// CallTool 调用远程工具执行接口。
func (c *Client) CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	var resp InvokeToolResponse
	code, err := c.doJSON(ctx, http.MethodPost, InvokePath, InvokeToolRequest{
		ServiceID: serviceID,
		ToolName:  name,
		Arguments: args,
		RequestID: newRequestID(),
	}, &resp)
	if err != nil {
		return nil, mcpclient.RuntimeStatus{}, err
	}
	if err := endpointError(code, resp.Error); err != nil {
		return resp.Result, resp.Status, err
	}
	return resp.Result, resp.Status, nil
}

func newRequestID() string {
	seq := rpcRequestCounter.Add(1)
	return fmt.Sprintf("rpc-%d-%d", time.Now().UnixNano(), seq)
}

// PingExecutor 调用远程探活接口。
func (c *Client) PingExecutor(ctx context.Context) error {
	var resp PingExecutorResponse
	code, err := c.doJSON(ctx, http.MethodGet, PingPath, nil, &resp)
	if err != nil {
		return err
	}
	return endpointError(code, resp.Error)
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, target any) (int, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return 0, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil && !errors.Is(err, io.EOF) {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func endpointError(statusCode int, message string) error {
	if message != "" {
		return errors.New(message)
	}
	if statusCode >= http.StatusBadRequest {
		return fmt.Errorf("rpc 请求失败: %d", statusCode)
	}
	return nil
}
