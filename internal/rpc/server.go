package rpc

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/pkg/logger"
)

// Executor 定义 RPC server 依赖的最小执行能力。
type Executor interface {
	Connect(ctx context.Context, service *entity.MCPService) (mcpclient.RuntimeStatus, error)
	Disconnect(ctx context.Context, serviceID string) error
	GetStatus(ctx context.Context, serviceID string) (mcpclient.RuntimeStatus, bool, error)
	ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error)
	CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error)
	Ping(ctx context.Context) error
}

// NewHandler 创建最小内部 RPC HTTP 处理器。
func NewHandler(executor Executor, opts ...ServerOption) http.Handler {
	server := &Server{executor: executor}
	for _, opt := range opts {
		opt(server)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(ConnectPath, server.handleConnect)
	mux.HandleFunc(DisconnectPath, server.handleDisconnect)
	mux.HandleFunc(StatusPath, server.handleStatus)
	mux.HandleFunc(ListToolsPath, server.handleListTools)
	mux.HandleFunc(InvokePath, server.handleInvoke)
	mux.HandleFunc(PingPath, server.handlePing)
	return mux
}

// Server 保存内部 RPC server 依赖。
type Server struct {
	executor   Executor
	executorID string
}

// ServerOption 定义 RPC server 选项。
type ServerOption func(*Server)

// WithExecutorID 注入由 executor 进程自身确定的身份标识。
func WithExecutorID(executorID string) ServerOption {
	return func(s *Server) {
		s.executorID = executorID
	}
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req ConnectServiceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ServiceSnapshot == nil {
		writeJSON(w, http.StatusBadRequest, ConnectServiceResponse{Error: "service_snapshot 不能为空"})
		return
	}
	status, err := s.executor.Connect(r.Context(), req.ServiceSnapshot)
	status = s.decorateStatus(status, req.RequestID)
	s.logOperation("connect", req.ServiceID, req.RequestID, err)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, ConnectServiceResponse{Status: status, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ConnectServiceResponse{Status: status})
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req DisconnectServiceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ServiceID == "" {
		writeJSON(w, http.StatusBadRequest, DisconnectServiceResponse{Error: "service_id 不能为空"})
		return
	}
	if err := s.executor.Disconnect(r.Context(), req.ServiceID); err != nil {
		s.logOperation("disconnect", req.ServiceID, req.RequestID, err)
		writeJSON(w, http.StatusBadGateway, DisconnectServiceResponse{
			ServiceID:  req.ServiceID,
			ExecutorID: s.executorID,
			RequestID:  req.RequestID,
			Error:      err.Error(),
		})
		return
	}
	s.logOperation("disconnect", req.ServiceID, req.RequestID, nil)
	writeJSON(w, http.StatusOK, DisconnectServiceResponse{
		ServiceID:  req.ServiceID,
		ExecutorID: s.executorID,
		RequestID:  req.RequestID,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req GetRuntimeStatusRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ServiceID == "" {
		writeJSON(w, http.StatusBadRequest, GetRuntimeStatusResponse{Error: "service_id 不能为空"})
		return
	}
	status, found, err := s.executor.GetStatus(r.Context(), req.ServiceID)
	status = s.decorateStatus(status, req.RequestID)
	s.logOperation("status", req.ServiceID, req.RequestID, err)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, GetRuntimeStatusResponse{Status: status, Found: found, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, GetRuntimeStatusResponse{Status: status, Found: found})
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req ListToolsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ServiceID == "" {
		writeJSON(w, http.StatusBadRequest, ListToolsResponse{Error: "service_id 不能为空"})
		return
	}
	tools, status, err := s.executor.ListTools(r.Context(), req.ServiceID)
	status = s.decorateStatus(status, req.RequestID)
	s.logOperation("list_tools", req.ServiceID, req.RequestID, err)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, ListToolsResponse{Tools: tools, Status: status, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ListToolsResponse{Tools: tools, Status: status})
}

func (s *Server) handleInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req InvokeToolRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ServiceID == "" {
		writeJSON(w, http.StatusBadRequest, InvokeToolResponse{Error: "service_id 不能为空"})
		return
	}
	if req.ToolName == "" {
		writeJSON(w, http.StatusBadRequest, InvokeToolResponse{Error: "tool_name 不能为空"})
		return
	}
	result, status, err := s.executor.CallTool(r.Context(), req.ServiceID, req.ToolName, req.Arguments)
	status = s.decorateStatus(status, req.RequestID)
	s.logOperation("invoke", req.ServiceID, req.RequestID, err)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, InvokeToolResponse{Result: result, Status: status, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, InvokeToolResponse{Result: result, Status: status})
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	if err := s.executor.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusBadGateway, PingExecutorResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, PingExecutorResponse{OK: true})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体 JSON 非法"})
		return false
	}
	return true
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) decorateStatus(status mcpclient.RuntimeStatus, requestID string) mcpclient.RuntimeStatus {
	if status.ServiceID == "" {
		return status
	}
	status.ExecutorID = s.executorID
	status.SnapshotWriter = s.executorID
	status.RequestID = requestID
	return status
}

func (s *Server) logOperation(operation, serviceID, requestID string, err error) {
	fields := []any{
		"executor_id", s.executorID,
		"request_id", requestID,
		"service_id", serviceID,
		"operation", operation,
	}
	if err != nil {
		fields = append(fields, "error", err)
		logger.S().Warnw("owner 诊断观测", fields...)
		return
	}
	logger.S().Infow("owner 诊断观测", fields...)
}
