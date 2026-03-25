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
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// ToolInvokeResult 定义调用结果
type ToolInvokeResult struct {
	Result     map[string]any `json:"result"`
	DurationMS int64          `json:"duration_ms"`
}

// ToolInvokeService 定义工具调用服务
type ToolInvokeService interface {
	Invoke(ctx context.Context, toolID string, arguments map[string]any, actor AuditEntry) (*ToolInvokeResult, error)
}

type toolInvokeService struct {
	cfg      config.HistoryConfig
	tools    repository.ToolRepository
	services repository.MCPServiceRepository
	history  repository.RequestHistoryRepository
	manager  *mcpclient.Manager
}

// NewToolInvokeService 创建工具调用服务
func NewToolInvokeService(cfg config.HistoryConfig, tools repository.ToolRepository, services repository.MCPServiceRepository, history repository.RequestHistoryRepository, manager *mcpclient.Manager) ToolInvokeService {
	return &toolInvokeService{cfg: cfg, tools: tools, services: services, history: history, manager: manager}
}

// Invoke 调用工具并记录请求历史
func (s *toolInvokeService) Invoke(ctx context.Context, toolID string, arguments map[string]any, actor AuditEntry) (*ToolInvokeResult, error) {
	tool, err := s.tools.GetByID(ctx, toolID)
	if err != nil {
		return nil, err
	}
	serviceItem, err := s.services.GetByID(ctx, tool.MCPServiceID)
	if err != nil {
		return nil, err
	}
	if serviceItem.Status == entity.ServiceStatusError {
		return nil, response.NewBizError(http.StatusConflict, response.CodeConflict, "服务处于错误状态，请先恢复连接", nil)
	}
	start := time.Now()
	result, runtimeStatus, err := s.manager.CallTool(ctx, tool.MCPServiceID, tool.Name, arguments)
	duration := time.Since(start).Milliseconds()

	// 无论调用成功还是失败，都先构造统一的历史记录对象
	requestBody, requestTruncated, requestHash, requestSize := sanitizeMap(arguments, s.cfg.MaxBodyBytes)
	responseBody := mcpclient.MarshalResult(result)
	responseClean, responseTruncated, responseHash, responseSize := sanitizeMap(responseBody, s.cfg.MaxBodyBytes)

	history := &entity.RequestHistory{
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
	if err != nil {
		history.Status = entity.RequestStatusFailed
		history.ErrorMessage = err.Error()
		if saveErr := s.history.Create(ctx, history); saveErr != nil {
			return nil, saveErr
		}
		return nil, normalizeToolActionError(ctx, s.services, serviceItem, "工具调用失败", err)
	}
	if result != nil && result.IsError {
		history.Status = entity.RequestStatusFailed
		history.ErrorMessage = "tool reported error"
	}
	if saveErr := s.history.Create(ctx, history); saveErr != nil {
		return nil, saveErr
	}
	return &ToolInvokeResult{
		Result: map[string]any{
			"transport_type": runtimeStatus.TransportType,
			"payload":        responseClean,
		},
		DurationMS: duration,
	}, nil
}

// sanitizeMap 对请求或响应体做脱敏、截断和摘要计算
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
	// 超长内容只保留前 maxBytes 字节的可解析部分，同时保留完整摘要
	var truncated map[string]any
	_ = json.Unmarshal(raw[:maxBytes], &truncated)
	if truncated == nil {
		truncated = map[string]any{"truncated": true}
	}
	return truncated, true, hex.EncodeToString(sum[:]), size
}

// deepSanitize 递归脱敏敏感字段
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

// isMap 判断值是否为 map[string]any
func isMap(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}
