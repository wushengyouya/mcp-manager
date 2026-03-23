package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
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
// @Router /api/v1/audit-logs [get]
func (h *AuditHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	items, total, err := h.audit.List(c.Request.Context(), repository.AuditListFilter{
		Page:         page,
		PageSize:     pageSize,
		UserID:       c.Query("user_id"),
		Action:       c.Query("action"),
		ResourceType: c.Query("resource_type"),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Page(c, items, page, pageSize, total)
}

// Export godoc
// @Summary 导出审计日志 CSV
// @Tags audit
// @Produce text/csv
// @Param action query string false "操作类型"
// @Success 200 {string} string "csv content"
// @Security BearerAuth
// @Router /api/v1/audit-logs/export [get]
func (h *AuditHandler) Export(c *gin.Context) {
	data, err := h.audit.ExportCSV(c.Request.Context(), repository.AuditListFilter{
		Action: c.Query("action"),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=audit_logs.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", data)
}
