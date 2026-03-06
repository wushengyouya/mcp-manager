package entity

import "time"

// RequestStatus 定义调用状态。
type RequestStatus string

const (
	// RequestStatusSuccess 表示成功。
	RequestStatusSuccess RequestStatus = "success"
	// RequestStatusFailed 表示失败。
	RequestStatusFailed RequestStatus = "failed"
)

// RequestHistory 定义调用历史实体。
type RequestHistory struct {
	ID                string        `gorm:"type:varchar(36);primaryKey" json:"id"`
	MCPServiceID      string        `gorm:"type:varchar(36);index;not null" json:"mcp_service_id"`
	ToolName          string        `gorm:"type:varchar(100);index;not null" json:"tool_name"`
	UserID            string        `gorm:"type:varchar(36);index;not null" json:"user_id"`
	RequestBody       JSONMap       `gorm:"type:json" json:"request_body"`
	ResponseBody      JSONMap       `gorm:"type:json" json:"response_body"`
	RequestTruncated  bool          `gorm:"default:false" json:"request_truncated"`
	ResponseTruncated bool          `gorm:"default:false" json:"response_truncated"`
	RequestHash       string        `gorm:"type:varchar(128)" json:"request_hash"`
	ResponseHash      string        `gorm:"type:varchar(128)" json:"response_hash"`
	RequestSize       int           `json:"request_size"`
	ResponseSize      int           `json:"response_size"`
	CompressionType   string        `gorm:"type:varchar(20);default:none" json:"compression_type"`
	Status            RequestStatus `gorm:"type:varchar(20);not null" json:"status"`
	ErrorMessage      string        `gorm:"type:text" json:"error_message"`
	DurationMS        int64         `json:"duration_ms"`
	CreatedAt         time.Time     `gorm:"autoCreateTime" json:"created_at"`
}
