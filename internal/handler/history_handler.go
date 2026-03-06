package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/middleware"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// HistoryHandler 定义历史处理器。
type HistoryHandler struct {
	repo repository.RequestHistoryRepository
}

// NewHistoryHandler 创建历史处理器。
func NewHistoryHandler(repo repository.RequestHistoryRepository) *HistoryHandler {
	return &HistoryHandler{repo: repo}
}

// List godoc
// @Summary 查询调用历史列表
// @Tags history
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页大小"
// @Param service_id query string false "服务ID"
// @Param tool_name query string false "工具名"
// @Param status query string false "状态"
// @Param start_at query string false "开始时间 RFC3339"
// @Param end_at query string false "结束时间 RFC3339"
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/history [get]
// List 查询历史列表。
func (h *HistoryHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	userID, _, role := middleware.CurrentUser(c)
	filter := repository.HistoryListFilter{
		Page:      page,
		PageSize:  pageSize,
		ServiceID: c.Query("service_id"),
		ToolName:  c.Query("tool_name"),
		Status:    c.Query("status"),
		UserID:    userID,
		IsAdmin:   role == entity.RoleAdmin,
	}
	if raw := c.Query("start_at"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			filter.StartAt = &t
		}
	}
	if raw := c.Query("end_at"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			filter.EndAt = &t
		}
	}
	items, total, err := h.repo.List(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Page(c, items, page, pageSize, total)
}

// Get godoc
// @Summary 查询调用历史详情
// @Tags history
// @Produce json
// @Param id path string true "历史ID"
// @Success 200 {object} response.Body
// @Failure 403 {object} response.Body
// @Failure 404 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/history/{id} [get]
// Get 查询历史详情。
func (h *HistoryHandler) Get(c *gin.Context) {
	item, err := h.repo.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	userID, _, role := middleware.CurrentUser(c)
	if role != entity.RoleAdmin && item.UserID != userID {
		response.Fail(c, http.StatusForbidden, response.CodeForbidden, "权限不足")
		return
	}
	response.Success(c, item)
}
