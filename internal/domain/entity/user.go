package entity

import "time"

// Role 定义用户角色。
type Role string

const (
	// RoleAdmin 表示管理员。
	RoleAdmin Role = "admin"
	// RoleOperator 表示操作员。
	RoleOperator Role = "operator"
	// RoleReadonly 表示只读用户。
	RoleReadonly Role = "readonly"
)

// User 定义系统用户实体。
type User struct {
	Base
	Username     string     `gorm:"type:varchar(50);uniqueIndex;not null" json:"username"`
	Password     string     `gorm:"type:varchar(100);not null" json:"-"`
	Email        string     `gorm:"type:varchar(100);uniqueIndex;not null" json:"email"`
	Role         Role       `gorm:"type:varchar(20);not null" json:"role"`
	IsActive     bool       `gorm:"default:true" json:"is_active"`
	IsFirstLogin bool       `gorm:"default:true" json:"is_first_login"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// CanModify 判断用户是否具备修改权限。
func (u User) CanModify() bool {
	return u.Role == RoleAdmin || u.Role == RoleOperator
}

// IsAdmin 判断用户是否为管理员。
func (u User) IsAdmin() bool {
	return u.Role == RoleAdmin
}
