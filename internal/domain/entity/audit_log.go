package entity

import "time"

// AuditLog 定义审计日志实体
type AuditLog struct {
	ID           string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	UserID       string    `gorm:"type:varchar(36);index" json:"user_id"`
	Username     string    `gorm:"type:varchar(50);not null" json:"username"`
	Action       string    `gorm:"type:varchar(50);index;not null" json:"action"`
	ResourceType string    `gorm:"type:varchar(50);index;not null" json:"resource_type"`
	ResourceID   string    `gorm:"type:varchar(36);index" json:"resource_id"`
	Detail       JSONMap   `gorm:"type:json" json:"detail"`
	IPAddress    string    `gorm:"type:varchar(45)" json:"ip_address"`
	UserAgent    string    `gorm:"type:varchar(500)" json:"user_agent"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}
