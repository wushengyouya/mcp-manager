package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// ToolInvokeResult 定义调用结果。
type ToolInvokeResult struct {
	Result     map[string]any `json:"result"`
	DurationMS int64          `json:"duration_ms"`
}

// ToolInvokeService 定义工具调用服务。
type ToolInvokeService interface {
	Invoke(ctx context.Context, toolID string, arguments map[string]any, actor AuditEntry) (*ToolInvokeResult, error)
	InvokeAsync(ctx context.Context, toolID string, arguments map[string]any, timeout time.Duration, actor AuditEntry) (*AsyncInvokeTask, error)
	GetTask(ctx context.Context, taskID string, actor AuditEntry) (*AsyncInvokeTask, error)
	CancelTask(ctx context.Context, taskID string, actor AuditEntry) (*AsyncInvokeTask, error)
	TaskStats(ctx context.Context, actor AuditEntry) (*AsyncTaskStats, error)
	Stop(ctx context.Context) error
}

// ToolInvokeOption 定义工具调用服务构造选项。
type ToolInvokeOption func(*toolInvokeService)

// WithToolInvokeExecutionConfig 注入执行治理配置。
func WithToolInvokeExecutionConfig(cfg config.ExecutionConfig) ToolInvokeOption {
	return func(s *toolInvokeService) {
		s.execution = cfg
		s.controller = NewInvokeController(cfg)
	}
}

// WithToolInvokeHistorySink 覆盖历史记录 sink。
func WithToolInvokeHistorySink(sink HistorySink) ToolInvokeOption {
	return func(s *toolInvokeService) {
		if sink != nil {
			s.historySink = sink
		}
	}
}

// WithToolInvokeController 覆盖执行治理控制器。
func WithToolInvokeController(controller *InvokeController) ToolInvokeOption {
	return func(s *toolInvokeService) {
		if controller == nil {
			return
		}
		s.controller = controller
	}
}

// toolInvokeService 实现工具调用服务。
type toolInvokeService struct {
	cfg         config.HistoryConfig
	execution   config.ExecutionConfig
	tools       repository.ToolRepository
	services    repository.MCPServiceRepository
	historySink HistorySink
	invoker     ToolInvoker
	controller  *InvokeController
	asyncTasks  *asyncTaskManager
}

// NewToolInvokeService 创建工具调用服务。
func NewToolInvokeService(cfg config.HistoryConfig, tools repository.ToolRepository, services repository.MCPServiceRepository, history repository.RequestHistoryRepository, invoker ToolInvoker, opts ...ToolInvokeOption) ToolInvokeService {
	svc := &toolInvokeService{
		cfg:         cfg,
		tools:       tools,
		services:    services,
		historySink: NewDBHistorySink(history),
		invoker:     invoker,
		controller:  NewInvokeController(config.ExecutionConfig{}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	if svc.asyncTasks == nil && svc.execution.AsyncInvokeEnabled {
		svc.asyncTasks = newAsyncTaskManager(svc.execution, svc.controller)
	}
	return svc
}

// Invoke 同步调用工具并记录请求历史。
func (s *toolInvokeService) Invoke(ctx context.Context, toolID string, arguments map[string]any, actor AuditEntry) (*ToolInvokeResult, error) {
	tool, serviceItem, err := s.loadTarget(ctx, toolID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureAllowed(serviceItem.ID, actor.UserID); err != nil {
		return nil, err
	}
	release, err := s.controller.Acquire(ctx)
	if err != nil {
		return nil, concurrencyLimitedError()
	}
	defer release()
	return s.invokeCore(ctx, tool, serviceItem, arguments, actor)
}

// InvokeAsync 异步提交工具调用任务。
func (s *toolInvokeService) InvokeAsync(ctx context.Context, toolID string, arguments map[string]any, timeout time.Duration, actor AuditEntry) (*AsyncInvokeTask, error) {
	if s.asyncTasks == nil {
		return nil, response.NewBizError(http.StatusConflict, response.CodeConflict, "异步调用未启用", nil)
	}
	tool, serviceItem, err := s.loadTarget(ctx, toolID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureAllowed(serviceItem.ID, actor.UserID); err != nil {
		return nil, err
	}
	return s.asyncTasks.submit(tool.ID, serviceItem.ID, arguments, timeout, actor, func(runCtx context.Context) (*ToolInvokeResult, error) {
		return s.invokeCore(runCtx, tool, serviceItem, arguments, actor)
	})
}

// GetTask 查询异步任务状态。
func (s *toolInvokeService) GetTask(_ context.Context, taskID string, actor AuditEntry) (*AsyncInvokeTask, error) {
	if s.asyncTasks == nil {
		return nil, response.NewBizError(http.StatusNotFound, response.CodeNotFound, "异步任务未启用", nil)
	}
	return s.asyncTasks.get(taskID, actor)
}

// CancelTask 取消异步任务。
func (s *toolInvokeService) CancelTask(_ context.Context, taskID string, actor AuditEntry) (*AsyncInvokeTask, error) {
	if s.asyncTasks == nil {
		return nil, response.NewBizError(http.StatusNotFound, response.CodeNotFound, "异步任务未启用", nil)
	}
	return s.asyncTasks.cancel(taskID, actor)
}

// TaskStats 返回异步任务总览。
func (s *toolInvokeService) TaskStats(_ context.Context, actor AuditEntry) (*AsyncTaskStats, error) {
	if s.asyncTasks == nil {
		return nil, response.NewBizError(http.StatusNotFound, response.CodeNotFound, "异步任务未启用", nil)
	}
	return s.asyncTasks.stats(actor)
}

// Stop 停止异步任务 worker。
func (s *toolInvokeService) Stop(ctx context.Context) error {
	if s.asyncTasks == nil {
		return nil
	}
	return s.asyncTasks.stop(ctx)
}

func (s *toolInvokeService) loadTarget(ctx context.Context, toolID string) (*entity.Tool, *entity.MCPService, error) {
	tool, err := s.tools.GetByID(ctx, toolID)
	if err != nil {
		return nil, nil, err
	}
	serviceItem, err := s.services.GetByID(ctx, tool.MCPServiceID)
	if err != nil {
		return nil, nil, err
	}
	if serviceItem.Status == entity.ServiceStatusError {
		return nil, nil, response.NewBizError(http.StatusConflict, response.CodeConflict, "服务处于错误状态，请先恢复连接", nil)
	}
	return tool, serviceItem, nil
}

func (s *toolInvokeService) ensureAllowed(serviceID, userID string) error {
	decision := s.controller.Allow(serviceID, userID)
	if decision.Allowed {
		return nil
	}
	return response.NewBizError(http.StatusTooManyRequests, response.CodeTooManyRequests, decision.Reason, ErrInvokeRateLimited)
}

func (s *toolInvokeService) invokeCore(ctx context.Context, tool *entity.Tool, serviceItem *entity.MCPService, arguments map[string]any, actor AuditEntry) (*ToolInvokeResult, error) {
	start := time.Now()
	result, runtimeStatus, err := s.invoker.CallTool(ctx, tool.MCPServiceID, tool.Name, arguments)
	duration := time.Since(start).Milliseconds()
	history := s.buildHistory(tool, actor, arguments, result, duration)
	if err != nil {
		history.Status = entity.RequestStatusFailed
		history.ErrorMessage = err.Error()
		if saveErr := s.historySink.Record(ctx, history); saveErr != nil {
			return nil, saveErr
		}
		return nil, normalizeToolActionError(ctx, s.services, serviceItem, "工具调用失败", err)
	}
	if result != nil && result.IsError {
		history.Status = entity.RequestStatusFailed
		history.ErrorMessage = "tool reported error"
	}
	if saveErr := s.historySink.Record(ctx, history); saveErr != nil {
		return nil, saveErr
	}
	return &ToolInvokeResult{
		Result: map[string]any{
			"transport_type": runtimeStatus.TransportType,
			"payload":        mcpclient.MarshalResult(result),
		},
		DurationMS: duration,
	}, nil
}

func (s *toolInvokeService) buildHistory(tool *entity.Tool, actor AuditEntry, arguments map[string]any, result *mcp.CallToolResult, duration int64) *entity.RequestHistory {
	requestBody, requestTruncated, requestHash, requestSize := sanitizeMap(arguments, s.cfg.MaxBodyBytes)
	responseBody := mcpclient.MarshalResult(result)
	responseClean, responseTruncated, responseHash, responseSize := sanitizeMap(responseBody, s.cfg.MaxBodyBytes)
	return &entity.RequestHistory{
		ID:                uuid.NewString(),
		MCPServiceID:      tool.MCPServiceID,
		ToolName:          tool.Name,
		UserID:            actor.UserID,
		RequestBody:       entity.JSONMap(requestBody),
		ResponseBody:      entity.JSONMap(responseClean),
		RequestTruncated:  requestTruncated,
		ResponseTruncated: responseTruncated,
		RequestHash:       requestHash,
		ResponseHash:      responseHash,
		RequestSize:       requestSize,
		ResponseSize:      responseSize,
		CompressionType:   s.cfg.Compression,
		Status:            entity.RequestStatusSuccess,
		DurationMS:        duration,
		CreatedAt:         time.Now(),
	}
}

func concurrencyLimitedError() error {
	return response.NewBizError(http.StatusTooManyRequests, response.CodeTooManyRequests, "执行并发已达上限，请稍后重试", ErrInvokeConcurrencyLimited)
}

// sanitizeMap 对请求或响应体做脱敏、截断和摘要计算。
func sanitizeMap(in map[string]any, maxBytes int) (map[string]any, bool, string, int) {
	if in == nil {
		return nil, false, "", 0
	}
	copyMap := deepSanitize(in)
	raw, _ := json.Marshal(copyMap)
	sum := sha256.Sum256(raw)
	if maxBytes <= 0 {
		maxBytes = 8192
	}
	size := len(raw)
	if size <= maxBytes {
		return copyMap, false, hex.EncodeToString(sum[:]), size
	}
	var truncated map[string]any
	_ = json.Unmarshal(raw[:maxBytes], &truncated)
	if truncated == nil {
		truncated = map[string]any{"truncated": true}
	}
	return truncated, true, hex.EncodeToString(sum[:]), size
}

// deepSanitize 递归脱敏敏感字段。
func deepSanitize(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		lk := strings.ToLower(k)
		switch {
		case lk == "authorization", lk == "password", lk == "secret", strings.Contains(lk, "token"):
			out[k] = "***"
		case isMap(v):
			out[k] = deepSanitize(v.(map[string]any))
		default:
			out[k] = v
		}
	}
	return out
}

// isMap 判断值是否为 map[string]any。
func isMap(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}
