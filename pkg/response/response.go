package response

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Body 定义统一响应结构。
type Body struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// PageData 定义分页数据结构。
type PageData struct {
	Items    any   `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

// Success 返回成功响应。
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: CodeSuccess, Message: "success", Data: data, Timestamp: time.Now().UnixMilli()})
}

// Created 返回创建成功响应。
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Body{Code: CodeSuccess, Message: "success", Data: data, Timestamp: time.Now().UnixMilli()})
}

// Page 返回分页响应。
func Page(c *gin.Context, items any, page, pageSize int, total int64) {
	Success(c, PageData{Items: items, Page: page, PageSize: pageSize, Total: total})
}

// Fail 返回失败响应。
func Fail(c *gin.Context, httpStatus, code int, message string) {
	c.JSON(httpStatus, Body{Code: code, Message: message, Timestamp: time.Now().UnixMilli()})
}

// Error 根据错误类型返回响应。
func Error(c *gin.Context, err error) {
	var bizErr *BizError
	if errors.As(err, &bizErr) {
		Fail(c, bizErr.HTTPStatus, bizErr.Code, bizErr.Message)
		return
	}
	Fail(c, http.StatusInternalServerError, CodeInternal, "internal error")
}
