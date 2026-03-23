package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/handler/dto"
	"github.com/mikasa/mcp-manager/internal/middleware"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// AuthHandler 定义认证处理器
type AuthHandler struct {
	auth service.AuthService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(auth service.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// Login godoc
// @Summary 用户登录
// @Tags auth
// @Accept json
// @Produce json
// @Param body body dto.LoginRequest true "登录请求"
// @Success 200 {object} response.Body
// @Failure 400 {object} response.Body
// @Failure 401 {object} response.Body
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeInvalidArgument, err.Error())
		return
	}
	pair, user, err := h.auth.Login(c.Request.Context(), req.Username, req.Password, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
		"user": gin.H{
			"id":             user.ID,
			"username":       user.Username,
			"email":          user.Email,
			"role":           user.Role,
			"is_first_login": user.IsFirstLogin,
		},
	})
}

// Logout godoc
// @Summary 用户登出
// @Tags auth
// @Accept json
// @Produce json
// @Param body body dto.LogoutRequest false "登出请求"
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	var req dto.LogoutRequest
	_ = c.ShouldBindJSON(&req)
	header := c.GetHeader("Authorization")
	accessToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	userID, username, _ := middleware.CurrentUser(c)
	if err := h.auth.Logout(c.Request.Context(), accessToken, req.RefreshToken, userID, username, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// Refresh godoc
// @Summary 刷新令牌
// @Tags auth
// @Accept json
// @Produce json
// @Param body body dto.RefreshTokenRequest true "刷新请求"
// @Success 200 {object} response.Body
// @Failure 401 {object} response.Body
// @Router /api/v1/auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeInvalidArgument, err.Error())
		return
	}
	pair, err := h.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
	})
}

// ChangePassword godoc
// @Summary 修改密码
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Param body body dto.ChangePasswordRequest true "修改密码请求"
// @Success 200 {object} response.Body
// @Failure 400 {object} response.Body
// @Failure 403 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/users/{id}/password [put]
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeInvalidArgument, err.Error())
		return
	}
	userID := c.Param("id")
	currentUserID, username, role := middleware.CurrentUser(c)
	if currentUserID != userID && role != "admin" {
		response.Fail(c, http.StatusForbidden, response.CodeForbidden, "只能修改自己的密码")
		return
	}
	if err := h.auth.ChangePassword(c.Request.Context(), userID, req.OldPassword, req.NewPassword, username, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
