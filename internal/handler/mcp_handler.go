package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/handler/dto"
	"github.com/mikasa/mcp-manager/internal/middleware"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// MCPHandler 定义服务处理器。
type MCPHandler struct {
	services service.MCPService
}

// NewMCPHandler 创建服务处理器。
func NewMCPHandler(services service.MCPService) *MCPHandler {
	return &MCPHandler{services: services}
}

func (h *MCPHandler) actor(c *gin.Context) service.AuditEntry {
	userID, username, _ := middleware.CurrentUser(c)
	return service.AuditEntry{UserID: userID, Username: username, IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent()}
}

func (h *MCPHandler) bindInput(c *gin.Context) (*service.CreateMCPServiceInput, bool) {
	var req dto.UpsertServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeInvalidArgument, err.Error())
		return nil, false
	}
	return &service.CreateMCPServiceInput{
		Name:          req.Name,
		Description:   req.Description,
		TransportType: entity.TransportType(req.TransportType),
		Command:       req.Command,
		Args:          req.Args,
		Env:           req.Env,
		URL:           req.URL,
		BearerToken:   req.BearerToken,
		CustomHeaders: req.CustomHeaders,
		SessionMode:   req.SessionMode,
		CompatMode:    req.CompatMode,
		ListenEnabled: req.ListenEnabled,
		Timeout:       req.Timeout,
		Tags:          req.Tags,
	}, true
}

// Create godoc
// @Summary 创建 MCP 服务
// @Tags services
// @Accept json
// @Produce json
// @Param body body dto.UpsertServiceRequest true "服务配置"
// @Success 201 {object} response.Body
// @Failure 400 {object} response.Body
// @Failure 409 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/services [post]
// Create 创建服务。
func (h *MCPHandler) Create(c *gin.Context) {
	input, ok := h.bindInput(c)
	if !ok {
		return
	}
	item, err := h.services.Create(c.Request.Context(), *input, h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, item)
}

// Update godoc
// @Summary 更新 MCP 服务
// @Tags services
// @Accept json
// @Produce json
// @Param id path string true "服务ID"
// @Param body body dto.UpsertServiceRequest true "服务配置"
// @Success 200 {object} response.Body
// @Failure 400 {object} response.Body
// @Failure 404 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/services/{id} [put]
// Update 更新服务。
func (h *MCPHandler) Update(c *gin.Context) {
	input, ok := h.bindInput(c)
	if !ok {
		return
	}
	item, err := h.services.Update(c.Request.Context(), c.Param("id"), *input, h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, item)
}

// Delete godoc
// @Summary 删除 MCP 服务
// @Tags services
// @Produce json
// @Param id path string true "服务ID"
// @Success 200 {object} response.Body
// @Failure 400 {object} response.Body
// @Failure 404 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/services/{id} [delete]
// Delete 删除服务。
func (h *MCPHandler) Delete(c *gin.Context) {
	if err := h.services.Delete(c.Request.Context(), c.Param("id"), h.actor(c)); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// Get godoc
// @Summary 获取 MCP 服务详情
// @Tags services
// @Produce json
// @Param id path string true "服务ID"
// @Success 200 {object} response.Body
// @Failure 404 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/services/{id} [get]
// Get 获取服务详情。
func (h *MCPHandler) Get(c *gin.Context) {
	item, err := h.services.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, item)
}

// List godoc
// @Summary 查询 MCP 服务列表
// @Tags services
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页大小"
// @Param transport_type query string false "传输类型"
// @Param tag query string false "标签"
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/services [get]
// List 查询服务列表。
func (h *MCPHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	items, total, err := h.services.List(c.Request.Context(), repository.MCPServiceListFilter{
		Page:          page,
		PageSize:      pageSize,
		TransportType: c.Query("transport_type"),
		Tag:           c.Query("tag"),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Page(c, items, page, pageSize, total)
}

// Connect godoc
// @Summary 连接 MCP 服务
// @Tags services
// @Produce json
// @Param id path string true "服务ID"
// @Success 200 {object} response.Body
// @Failure 404 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/services/{id}/connect [post]
// Connect 连接服务。
func (h *MCPHandler) Connect(c *gin.Context) {
	status, err := h.services.Connect(c.Request.Context(), c.Param("id"), h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, status)
}

// Disconnect godoc
// @Summary 断开 MCP 服务
// @Tags services
// @Produce json
// @Param id path string true "服务ID"
// @Success 200 {object} response.Body
// @Failure 404 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/services/{id}/disconnect [post]
// Disconnect 断开服务。
func (h *MCPHandler) Disconnect(c *gin.Context) {
	if err := h.services.Disconnect(c.Request.Context(), c.Param("id"), h.actor(c)); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// Status godoc
// @Summary 查询 MCP 服务状态
// @Tags services
// @Produce json
// @Param id path string true "服务ID"
// @Success 200 {object} response.Body
// @Failure 404 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/services/{id}/status [get]
// Status 查询运行状态。
func (h *MCPHandler) Status(c *gin.Context) {
	status, err := h.services.Status(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, status)
}
