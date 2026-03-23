package dto

// InvokeToolRequest 定义工具调用请求
type InvokeToolRequest struct {
	Arguments map[string]any `json:"arguments" binding:"required"`
}
