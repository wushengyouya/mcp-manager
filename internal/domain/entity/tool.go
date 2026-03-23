package entity

import "time"

// Tool 定义工具元数据实体
type Tool struct {
	Base
	MCPServiceID string    `gorm:"type:varchar(36);index;not null;index:idx_service_tool_active,unique,priority:1,where:deleted_at IS NULL" json:"mcp_service_id"`
	Name         string    `gorm:"type:varchar(100);not null;index:idx_service_tool_active,unique,priority:2,where:deleted_at IS NULL" json:"name"`
	Description  string    `gorm:"type:text" json:"description"`
	InputSchema  JSONMap   `gorm:"type:json" json:"input_schema"`
	IsEnabled    bool      `gorm:"default:true" json:"is_enabled"`
	SyncedAt     time.Time `json:"synced_at"`
}
