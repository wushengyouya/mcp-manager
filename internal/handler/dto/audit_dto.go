package dto

// AuditListQuery 定义审计列表查询参数
type AuditListQuery struct {
	PageQuery
	UserID       string `form:"user_id"`
	Action       string `form:"action"`
	ResourceType string `form:"resource_type"`
}

// AuditExportQuery 定义审计导出查询
type AuditExportQuery struct {
	Action string `form:"action"`
}
