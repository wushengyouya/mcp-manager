package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/handler/dto"
	"github.com/mikasa/mcp-manager/internal/middleware"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// HistoryHandler 定义历史处理器
type HistoryHandler struct {
	repo repository.RequestHistoryRepository
}

// NewHistoryHandler 创建历史处理器
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
// @Router /history [get]
func (h *HistoryHandler) List(c *gin.Context) {
	var query dto.HistoryListQuery
	if !bindQuery(c, &query) {
		return
	}
	userID, _, role := middleware.CurrentUser(c)
	filter := repository.HistoryListFilter{
		Page:      query.GetPage(),
		PageSize:  query.GetPageSize(),
		ServiceID: query.ServiceID,
		ToolName:  query.ToolName,
		Status:    query.Status,
		UserID:    userID,
		IsAdmin:   role == entity.RoleAdmin,
	}
	startAt, ok := parseRFC3339(c, "start_at", query.StartAt)
	if !ok {
		return
	}
	endAt, ok := parseRFC3339(c, "end_at", query.EndAt)
	if !ok {
		return
	}
	filter.StartAt = startAt
	filter.EndAt = endAt
	items, total, err := h.repo.List(c.Request.Context(), filter)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Page(c, items, query.GetPage(), query.GetPageSize(), total)
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
// @Router /history/{id} [get]
func (h *HistoryHandler) Get(c *gin.Context) {
	var path dto.IDPathRequest
	if !bindURI(c, &path) {
		return
	}
	item, err := h.repo.GetByID(c.Request.Context(), path.ID)
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
