package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/response"
)

const (
	userIDKey   = "current_user_id"
	usernameKey = "current_username"
	roleKey     = "current_role"
)

// Auth 返回认证中间件。
func Auth(jwtSvc *appcrypto.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			response.Fail(c, http.StatusUnauthorized, response.CodeUnauthorized, "未提供有效的 bearer token")
			c.Abort()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		claims, err := jwtSvc.ParseToken(token, appcrypto.TokenTypeAccess)
		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				response.Fail(c, http.StatusUnauthorized, response.CodeTokenExpired, "token 已过期")
			} else {
				response.Fail(c, http.StatusUnauthorized, response.CodeUnauthorized, "token 无效")
			}
			c.Abort()
			return
		}
		c.Set(userIDKey, claims.UserID)
		c.Set(usernameKey, claims.Username)
		c.Set(roleKey, claims.Role)
		c.Next()
	}
}

// CurrentUser 返回当前登录用户信息。
func CurrentUser(c *gin.Context) (string, string, entity.Role) {
	userID, _ := c.Get(userIDKey)
	username, _ := c.Get(usernameKey)
	role, _ := c.Get(roleKey)
	return userID.(string), username.(string), entity.Role(role.(string))
}
