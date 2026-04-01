package handler

import (
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
	userID, username, _ := middleware.CurrentUser(c)
	return service.AuditEntry{UserID: userID, Username: username, IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent()}
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
