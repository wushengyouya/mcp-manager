package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/handler/dto"
	"github.com/mikasa/mcp-manager/internal/middleware"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// MCPHandler 定义服务处理器
type MCPHandler struct {
	services service.MCPService
}

// NewMCPHandler 创建服务处理器
func NewMCPHandler(services service.MCPService) *MCPHandler {
	return &MCPHandler{services: services}
}

// actor 构造当前请求对应的审计操作者信息
func (h *MCPHandler) actor(c *gin.Context) service.AuditEntry {
	userID, username, _ := middleware.CurrentUser(c)
	return service.AuditEntry{UserID: userID, Username: username, IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent()}
}

// bindInput 绑定并转换服务创建或更新请求
func (h *MCPHandler) bindInput(c *gin.Context) (*service.CreateMCPServiceInput, bool) {
	var req dto.UpsertServiceRequest
	if !bindJSON(c, &req) {
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
// @Router /services [post]
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
// @Router /services/{id} [put]
func (h *MCPHandler) Update(c *gin.Context) {
	input, ok := h.bindInput(c)
	if !ok {
		return
	}
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	item, err := h.services.Update(c.Request.Context(), path.ID, *input, h.actor(c))
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
// @Router /services/{id} [delete]
func (h *MCPHandler) Delete(c *gin.Context) {
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	if err := h.services.Delete(c.Request.Context(), path.ID, h.actor(c)); err != nil {
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
// @Router /services/{id} [get]
func (h *MCPHandler) Get(c *gin.Context) {
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	item, err := h.services.Get(c.Request.Context(), path.ID)
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
// @Router /services [get]
func (h *MCPHandler) List(c *gin.Context) {
	var query dto.ServiceListQuery
	if !bindQuery(c, &query) {
		return
	}
	items, total, err := h.services.List(c.Request.Context(), repository.MCPServiceListFilter{
		Page:          query.GetPage(),
		PageSize:      query.GetPageSize(),
		TransportType: query.TransportType,
		Tag:           query.Tag,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Page(c, items, query.GetPage(), query.GetPageSize(), total)
}

// Connect godoc
// @Summary 连接 MCP 服务
// @Tags services
// @Produce json
// @Param id path string true "服务ID"
// @Success 200 {object} response.Body
// @Failure 404 {object} response.Body
// @Security BearerAuth
// @Router /services/{id}/connect [post]
func (h *MCPHandler) Connect(c *gin.Context) {
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	status, err := h.services.Connect(c.Request.Context(), path.ID, h.actor(c))
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
// @Router /services/{id}/disconnect [post]
func (h *MCPHandler) Disconnect(c *gin.Context) {
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	if err := h.services.Disconnect(c.Request.Context(), path.ID, h.actor(c)); err != nil {
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
// @Router /services/{id}/status [get]
func (h *MCPHandler) Status(c *gin.Context) {
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	status, err := h.services.Status(c.Request.Context(), path.ID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, status)
}
