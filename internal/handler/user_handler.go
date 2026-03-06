package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/handler/dto"
	"github.com/mikasa/mcp-manager/internal/middleware"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// UserHandler 定义用户处理器。
type UserHandler struct {
	users service.UserService
	auth  service.AuthService
}

// NewUserHandler 创建用户处理器。
func NewUserHandler(users service.UserService, auth service.AuthService) *UserHandler {
	return &UserHandler{users: users, auth: auth}
}

func (h *UserHandler) actor(c *gin.Context) service.AuditEntry {
	userID, username, _ := middleware.CurrentUser(c)
	return service.AuditEntry{UserID: userID, Username: username, IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent()}
}

// Create godoc
// @Summary 创建用户
// @Tags users
// @Accept json
// @Produce json
// @Param body body dto.CreateUserRequest true "创建用户请求"
// @Success 201 {object} response.Body
// @Failure 400 {object} response.Body
// @Failure 409 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/users [post]
// Create 创建用户。
func (h *UserHandler) Create(c *gin.Context) {
	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeInvalidArgument, err.Error())
		return
	}
	user, err := h.users.Create(c.Request.Context(), service.CreateUserInput{
		Username: req.Username,
		Password: req.Password,
		Email:    req.Email,
		Role:     entity.Role(req.Role),
	}, h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, user)
}

// Update godoc
// @Summary 更新用户
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "用户ID"
// @Param body body dto.UpdateUserRequest true "更新用户请求"
// @Success 200 {object} response.Body
// @Failure 400 {object} response.Body
// @Failure 404 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/users/{id} [put]
// Update 更新用户。
func (h *UserHandler) Update(c *gin.Context) {
	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, response.CodeInvalidArgument, err.Error())
		return
	}
	userID := c.Param("id")
	user, err := h.users.Update(c.Request.Context(), userID, service.UpdateUserInput{
		Email:    req.Email,
		Role:     entity.Role(req.Role),
		IsActive: req.IsActive,
	}, h.actor(c))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, user)
}

// Delete godoc
// @Summary 删除用户
// @Tags users
// @Produce json
// @Param id path string true "用户ID"
// @Success 200 {object} response.Body
// @Failure 400 {object} response.Body
// @Failure 404 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/users/{id} [delete]
// Delete 删除用户。
func (h *UserHandler) Delete(c *gin.Context) {
	currentUserID, _, _ := middleware.CurrentUser(c)
	if err := h.users.Delete(c.Request.Context(), c.Param("id"), currentUserID, h.actor(c)); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// List godoc
// @Summary 查询用户列表
// @Tags users
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页大小"
// @Param role query string false "角色"
// @Param active query bool false "是否启用"
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router /api/v1/users [get]
// List 查询用户列表。
func (h *UserHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	var active *bool
	if raw := c.Query("active"); raw != "" {
		value := raw == "true"
		active = &value
	}
	items, total, err := h.users.List(c.Request.Context(), repository.UserListFilter{
		Page:     page,
		PageSize: pageSize,
		Role:     c.Query("role"),
		Active:   active,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Page(c, items, page, pageSize, total)
}

// ChangePassword 修改密码。
func (h *UserHandler) ChangePassword(c *gin.Context) {
	authHandler := NewAuthHandler(h.auth)
	authHandler.ChangePassword(c)
}
