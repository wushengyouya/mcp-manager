package dto

// InvokeToolRequest 定义工具调用请求
type InvokeToolRequest struct {
	Arguments map[string]any `json:"arguments" binding:"required"`
}

// InvokeToolAsyncRequest 定义异步工具调用请求。
type InvokeToolAsyncRequest struct {
	Arguments map[string]any `json:"arguments" binding:"required"`
	TimeoutMS int            `json:"timeout_ms"`
}
