package dto

// CreateUserRequest 定义创建用户请求
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Role     string `json:"role" binding:"required,oneof=admin operator readonly"`
}

// UpdateUserRequest 定义更新用户请求
type UpdateUserRequest struct {
	Email    string `json:"email" binding:"omitempty,email"`
	Role     string `json:"role" binding:"omitempty,oneof=admin operator readonly"`
	IsActive *bool  `json:"is_active"`
}

// HasUpdates 判断是否至少提供了一个更新字段
func (r UpdateUserRequest) HasUpdates() bool {
	return r.Email != "" || r.Role != "" || r.IsActive != nil
}

// UserListQuery 定义用户列表查询参数
type UserListQuery struct {
	PageQuery
	Role   string `form:"role" binding:"omitempty,oneof=admin operator readonly"`
	Active *bool  `form:"active"`
}
