package mcpclient

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

// MarshalResult 将调用结果转为通用 JSON
func MarshalResult(result *mcp.CallToolResult) map[string]any {
	if result == nil {
		return nil
	}
	payload := map[string]any{
		"is_error": result.IsError,
	}
	if result.StructuredContent != nil {
		payload["structured_content"] = result.StructuredContent
	}
	if len(result.Content) > 0 {
		parts := make([]any, 0, len(result.Content))
		for _, item := range result.Content {
			raw, _ := json.Marshal(item)
			var decoded any
			_ = json.Unmarshal(raw, &decoded)
			parts = append(parts, decoded)
		}
		payload["content"] = parts
	}
	return payload
}
