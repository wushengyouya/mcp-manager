package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/handler/dto"
	"github.com/mikasa/mcp-manager/internal/middleware"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// ToolHandler 定义工具处理器。
type ToolHandler struct {
	tools  service.ToolService
	invoke service.ToolInvokeService
}

// NewToolHandler 创建工具处理器。
func NewToolHandler(tools service.ToolService, invoke service.ToolInvokeService) *ToolHandler {
	return &ToolHandler{tools: tools, invoke: invoke}
}

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
// @Router /api/v1/services/{id}/tools [get]
// ListByService 查询服务工具列表。
func (h *ToolHandler) ListByService(c *gin.Context) {
	items, err := h.tools.ListByService(c.Request.Context(), c.Param("id"))
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
// @Router /api/v1/tools/{id} [get]
// Get 查询工具详情。
func (h *ToolHandler) Get(c *gin.Context) {
	item, err := h.tools.Get(c.Request.Context(), c.Param("id"))
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
// @Router /api/v1/services/{id}/sync-tools [post]
// Sync 同步工具。
func (h *ToolHandler) Sync(c *gin.Context) {
	items, err := h.tools.Sync(c.Request.Context(), c.Param("id"), h.actor(c))
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
// @Router /api/v1/tools/{id}/invoke [post]
// Invoke 调用工具。
func (h *ToolHandler) Invoke(c *gin.Context) {
	var req dto.InvokeToolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeInvalidArgument, err.Error())
		return
	}
	result, err := h.invoke.Invoke(c.Request.Context(), c.Param("id"), req.Arguments, h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}
