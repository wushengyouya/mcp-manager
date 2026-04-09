package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/handler/dto"
	"github.com/mikasa/mcp-manager/internal/middleware"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// ToolHandler 定义工具处理器
type ToolHandler struct {
	tools  service.ToolService
	invoke service.ToolInvokeService
}

// NewToolHandler 创建工具处理器
func NewToolHandler(tools service.ToolService, invoke service.ToolInvokeService) *ToolHandler {
	return &ToolHandler{tools: tools, invoke: invoke}
}

// actor 构造当前请求对应的审计操作者信息
func (h *ToolHandler) actor(c *gin.Context) service.AuditEntry {
	userID, username, role := middleware.CurrentUser(c)
	return service.AuditEntry{UserID: userID, Username: username, Role: role, IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent()}
}

// ListByService godoc
// @Summary 查询服务工具列表
// @Tags tools
// @Produce json
// @Param id path string true "服务ID"
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router /services/{id}/tools [get]
func (h *ToolHandler) ListByService(c *gin.Context) {
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	items, err := h.tools.ListByService(c.Request.Context(), path.ID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, items)
}

// Get godoc
// @Summary 查询工具详情
// @Tags tools
// @Produce json
// @Param id path string true "工具ID"
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router /tools/{id} [get]
func (h *ToolHandler) Get(c *gin.Context) {
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	item, err := h.tools.Get(c.Request.Context(), path.ID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, item)
}

// Sync godoc
// @Summary 同步工具列表
// @Tags tools
// @Produce json
// @Param id path string true "服务ID"
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router /services/{id}/sync-tools [post]
func (h *ToolHandler) Sync(c *gin.Context) {
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	items, err := h.tools.Sync(c.Request.Context(), path.ID, h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, items)
}

// Invoke godoc
// @Summary 调用工具
// @Tags tools
// @Accept json
// @Produce json
// @Param id path string true "工具ID"
// @Param body body dto.InvokeToolRequest true "工具调用请求"
// @Success 200 {object} response.Body
// @Failure 400 {object} response.Body
// @Failure 502 {object} response.Body
// @Security BearerAuth
// @Router /tools/{id}/invoke [post]
func (h *ToolHandler) Invoke(c *gin.Context) {
	var req dto.InvokeToolRequest
	if !bindJSON(c, &req) {
		return
	}
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	result, err := h.invoke.Invoke(c.Request.Context(), path.ID, req.Arguments, h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

// InvokeAsync godoc
// @Summary 异步调用工具
// @Tags tools
// @Accept json
// @Produce json
// @Param id path string true "工具ID"
// @Param body body dto.InvokeToolAsyncRequest true "异步工具调用请求"
// @Success 202 {object} response.Body
// @Failure 400 {object} response.Body
// @Failure 502 {object} response.Body
// @Security BearerAuth
// @Router /tools/{id}/invoke-async [post]
func (h *ToolHandler) InvokeAsync(c *gin.Context) {
	var req dto.InvokeToolAsyncRequest
	if !bindJSON(c, &req) {
		return
	}
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	timeout := time.Duration(req.TimeoutMS) * time.Millisecond
	result, err := h.invoke.InvokeAsync(c.Request.Context(), path.ID, req.Arguments, timeout, h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Accepted(c, result)
}

// GetTask godoc
// @Summary 查询异步任务状态
// @Tags tools
// @Produce json
// @Param id path string true "任务ID"
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router /tasks/{id} [get]
func (h *ToolHandler) GetTask(c *gin.Context) {
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	result, err := h.invoke.GetTask(c.Request.Context(), path.ID, h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

// CancelTask godoc
// @Summary 取消异步任务
// @Tags tools
// @Produce json
// @Param id path string true "任务ID"
// @Success 202 {object} response.Body
// @Security BearerAuth
// @Router /tasks/{id}/cancel [post]
func (h *ToolHandler) CancelTask(c *gin.Context) {
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	result, err := h.invoke.CancelTask(c.Request.Context(), path.ID, h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Accepted(c, result)
}

// TaskStats godoc
// @Summary 查询异步任务总览
// @Tags tools
// @Produce json
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router /tasks/stats [get]
func (h *ToolHandler) TaskStats(c *gin.Context) {
	result, err := h.invoke.TaskStats(c.Request.Context(), h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}
