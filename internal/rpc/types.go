package rpc

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
)

const (
	ConnectPath    = "/internal/rpc/connect"
	DisconnectPath = "/internal/rpc/disconnect"
	StatusPath     = "/internal/rpc/status"
	ListToolsPath  = "/internal/rpc/list-tools"
	InvokePath     = "/internal/rpc/invoke"
	PingPath       = "/internal/rpc/ping"
)

// Actor 保存内部 RPC 调用发起者信息。
type Actor struct {
	UserID   string `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
}

// ConnectServiceRequest 定义远程连接请求。
type ConnectServiceRequest struct {
	ServiceID       string             `json:"service_id,omitempty"`
	ExpectedEpoch   int64              `json:"expected_epoch,omitempty"`
	ServiceSnapshot *entity.MCPService `json:"service_snapshot,omitempty"`
	Actor           Actor              `json:"actor,omitempty"`
}

// ConnectServiceResponse 定义远程连接响应。
type ConnectServiceResponse struct {
	Status mcpclient.RuntimeStatus `json:"status"`
	Error  string                  `json:"error,omitempty"`
}

// DisconnectServiceRequest 定义远程断开请求。
type DisconnectServiceRequest struct {
	ServiceID     string `json:"service_id"`
	ExpectedEpoch int64  `json:"expected_epoch,omitempty"`
	Actor         Actor  `json:"actor,omitempty"`
}

// DisconnectServiceResponse 定义远程断开响应。
type DisconnectServiceResponse struct {
	Error string `json:"error,omitempty"`
}

// GetRuntimeStatusRequest 定义远程状态请求。
type GetRuntimeStatusRequest struct {
	ServiceID string `json:"service_id"`
}

// GetRuntimeStatusResponse 定义远程状态响应。
type GetRuntimeStatusResponse struct {
	Status mcpclient.RuntimeStatus `json:"status"`
	Found  bool                    `json:"found"`
	Error  string                  `json:"error,omitempty"`
}

// ListToolsRequest 定义远程工具列表请求。
type ListToolsRequest struct {
	ServiceID string `json:"service_id"`
}

// ListToolsResponse 定义远程工具列表响应。
type ListToolsResponse struct {
	Tools  []mcp.Tool              `json:"tools,omitempty"`
	Status mcpclient.RuntimeStatus `json:"status"`
	Error  string                  `json:"error,omitempty"`
}

// InvokeToolRequest 定义远程工具调用请求。
type InvokeToolRequest struct {
	ServiceID     string         `json:"service_id"`
	ToolID        string         `json:"tool_id,omitempty"`
	ToolName      string         `json:"tool_name"`
	Arguments     map[string]any `json:"arguments,omitempty"`
	ExpectedEpoch int64          `json:"expected_epoch,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	TimeoutMS     int            `json:"timeout_ms,omitempty"`
	Actor         Actor          `json:"actor,omitempty"`
}

// InvokeToolResponse 定义远程工具调用响应。
type InvokeToolResponse struct {
	Result *mcp.CallToolResult     `json:"result,omitempty"`
	Status mcpclient.RuntimeStatus `json:"status"`
	Error  string                  `json:"error,omitempty"`
}

// PingExecutorResponse 定义 executor 探活响应。
type PingExecutorResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}
