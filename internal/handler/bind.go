package handler

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/mikasa/mcp-manager/pkg/response"
)

const optionalJSONBodyLimit = 8 * 1024

// failInvalidArgument 统一返回参数错误
func failInvalidArgument(c *gin.Context, err error) {
	response.Fail(c, http.StatusBadRequest, response.CodeInvalidArgument, err.Error())
}

// bindJSON 绑定 JSON 请求体
func bindJSON(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		failInvalidArgument(c, err)
		return false
	}
	return true
}

// bindOptionalJSON 允许空请求体，但不允许非法 JSON。
func bindOptionalJSON(c *gin.Context, req any) bool {
	reader := http.MaxBytesReader(c.Writer, c.Request.Body, optionalJSONBodyLimit)
	body, err := io.ReadAll(reader)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			failInvalidArgument(c, fmt.Errorf("请求体不能超过 %d 字节", optionalJSONBodyLimit))
			return false
		}
		failInvalidArgument(c, err)
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		return true
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return bindJSON(c, req)
}

// bindQuery 绑定查询参数
func bindQuery(c *gin.Context, req any) bool {
	clonedReq := c.Request.Clone(c.Request.Context())
	clonedURL := *c.Request.URL
	clonedURL.RawQuery = normalizeQueryValues(c.Request.URL.Query()).Encode()
	clonedReq.URL = &clonedURL

	if err := binding.Query.Bind(clonedReq, req); err != nil {
		failInvalidArgument(c, err)
		return false
	}
	return true
}

func normalizeQueryValues(values url.Values) url.Values {
	normalized := make(url.Values, len(values))
	for key, rawValues := range values {
		filtered := make([]string, 0, len(rawValues))
		for _, value := range rawValues {
			if value == "" {
				continue
			}
			filtered = append(filtered, value)
		}
		if len(filtered) > 0 {
			normalized[key] = filtered
		}
	}
	return normalized
}

// bindURI 绑定路径参数
func bindURI(c *gin.Context, req any) bool {
	if err := c.ShouldBindUri(req); err != nil {
		failInvalidArgument(c, err)
		return false
	}
	return true
}

// parseRFC3339 解析可选的 RFC3339 时间参数
func parseRFC3339(c *gin.Context, field, raw string) (*time.Time, bool) {
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		failInvalidArgument(c, fmt.Errorf("%s 必须符合 RFC3339 格式", field))
		return nil, false
	}
	return &t, true
}
