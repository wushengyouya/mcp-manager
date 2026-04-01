package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/handler/dto"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// AuditHandler 定义审计处理器
type AuditHandler struct {
	audit service.AuditService
}

// NewAuditHandler 创建审计处理器
func NewAuditHandler(audit service.AuditService) *AuditHandler {
	return &AuditHandler{audit: audit}
}

// List godoc
// @Summary 查询审计日志
// @Tags audit
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页大小"
// @Param user_id query string false "用户ID"
// @Param action query string false "操作类型"
// @Param resource_type query string false "资源类型"
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router /audit-logs [get]
func (h *AuditHandler) List(c *gin.Context) {
	var query dto.AuditListQuery
	if !bindQuery(c, &query) {
		return
	}
	items, total, err := h.audit.List(c.Request.Context(), repository.AuditListFilter{
		Page:         query.GetPage(),
		PageSize:     query.GetPageSize(),
		UserID:       query.UserID,
		Action:       query.Action,
		ResourceType: query.ResourceType,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Page(c, items, query.GetPage(), query.GetPageSize(), total)
}

// Export godoc
// @Summary 导出审计日志 CSV
// @Tags audit
// @Produce text/csv
// @Param action query string false "操作类型"
// @Success 200 {string} string "csv content"
// @Security BearerAuth
// @Router /audit-logs/export [get]
func (h *AuditHandler) Export(c *gin.Context) {
	var query dto.AuditExportQuery
	if !bindQuery(c, &query) {
		return
	}
	data, err := h.audit.ExportCSV(c.Request.Context(), repository.AuditListFilter{
		Action: query.Action,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=audit_logs.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", data)
}
