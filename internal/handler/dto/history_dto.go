package dto

// HistoryListQuery 定义历史列表查询参数
type HistoryListQuery struct {
	PageQuery
	ServiceID string `form:"service_id"`
	ToolName  string `form:"tool_name"`
	Status    string `form:"status"`
	StartAt   string `form:"start_at"`
	EndAt     string `form:"end_at"`
}
