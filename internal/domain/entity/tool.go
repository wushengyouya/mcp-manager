package entity

import "time"

// Tool 定义工具元数据实体
type Tool struct {
	Base
	MCPServiceID string    `gorm:"type:varchar(36);index;not null" json:"mcp_service_id"`
	Name         string    `gorm:"type:varchar(100);not null" json:"name"`
	Description  string    `gorm:"type:text" json:"description"`
	InputSchema  JSONMap   `json:"input_schema"`
	IsEnabled    bool      `json:"is_enabled"`
	SyncedAt     time.Time `json:"synced_at"`
}
