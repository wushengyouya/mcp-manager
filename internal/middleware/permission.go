package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// RequireRole 校验角色。
func RequireRole(roles ...entity.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, _, role := CurrentUser(c)
		for _, allowed := range roles {
			if role == allowed {
				c.Next()
				return
			}
		}
		response.Fail(c, http.StatusForbidden, response.CodeForbidden, "权限不足")
		c.Abort()
	}
}

// RequireAdmin 要求管理员。
func RequireAdmin() gin.HandlerFunc {
	return RequireRole(entity.RoleAdmin)
}

// RequireModify 要求修改权限。
func RequireModify() gin.HandlerFunc {
	return RequireRole(entity.RoleAdmin, entity.RoleOperator)
}
