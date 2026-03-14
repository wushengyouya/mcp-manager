package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// parseBody 从响应中解析 Body 结构。
func parseBody(t *testing.T, w *httptest.ResponseRecorder) Body {
	t.Helper()
	var body Body
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// TestSuccess 测试成功响应返回 200 和 code=0。
func TestSuccess(t *testing.T) {
	tests := []struct {
		name string
		data any
	}{
		{"nil_data", nil},
		{"string_data", "hello"},
		{"map_data", map[string]int{"count": 42}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			Success(c, tt.data)

			require.Equal(t, http.StatusOK, w.Code)
			body := parseBody(t, w)
			require.Equal(t, CodeSuccess, body.Code)
			require.Equal(t, "success", body.Message)
			require.NotZero(t, body.Timestamp)
		})
	}
}

// TestCreated 测试创建成功响应返回 201 和 code=0。
func TestCreated(t *testing.T) {
	tests := []struct {
		name string
		data any
	}{
		{"with_id", map[string]string{"id": "abc"}},
		{"nil_data", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			Created(c, tt.data)

			require.Equal(t, http.StatusCreated, w.Code)
			body := parseBody(t, w)
			require.Equal(t, CodeSuccess, body.Code)
			require.Equal(t, "success", body.Message)
			require.NotZero(t, body.Timestamp)
		})
	}
}

// TestPage 测试分页响应返回正确的分页结构。
func TestPage(t *testing.T) {
	tests := []struct {
		name     string
		items    any
		page     int
		pageSize int
		total    int64
	}{
		{"normal_page", []string{"a", "b"}, 1, 10, 50},
		{"empty_page", []string{}, 2, 20, 0},
		{"last_page", []int{1}, 5, 10, 41},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			Page(c, tt.items, tt.page, tt.pageSize, tt.total)

			require.Equal(t, http.StatusOK, w.Code)

			var raw map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))

			var pd PageData
			require.NoError(t, json.Unmarshal(raw["data"], &pd))
			require.Equal(t, tt.page, pd.Page)
			require.Equal(t, tt.pageSize, pd.PageSize)
			require.Equal(t, tt.total, pd.Total)
		})
	}
}

// TestFail 测试失败响应返回指定的 HTTP 状态码和业务码。
func TestFail(t *testing.T) {
	tests := []struct {
		name       string
		httpStatus int
		code       int
		message    string
	}{
		{"bad_request", http.StatusBadRequest, CodeInvalidArgument, "bad param"},
		{"not_found", http.StatusNotFound, CodeNotFound, "not found"},
		{"conflict", http.StatusConflict, CodeConflict, "duplicate"},
		{"forbidden", http.StatusForbidden, CodeForbidden, "no permission"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			Fail(c, tt.httpStatus, tt.code, tt.message)

			require.Equal(t, tt.httpStatus, w.Code)
			body := parseBody(t, w)
			require.Equal(t, tt.code, body.Code)
			require.Equal(t, tt.message, body.Message)
			require.NotZero(t, body.Timestamp)
		})
	}
}

// TestError_BizError 测试 Error 处理 BizError 时返回正确的状态码和业务码。
func TestError_BizError(t *testing.T) {
	tests := []struct {
		name       string
		bizErr     *BizError
		wantHTTP   int
		wantCode   int
		wantMsg    string
	}{
		{
			"not_found",
			NewBizError(http.StatusNotFound, CodeNotFound, "resource missing", nil),
			http.StatusNotFound, CodeNotFound, "resource missing",
		},
		{
			"unauthorized_with_cause",
			NewBizError(http.StatusUnauthorized, CodeUnauthorized, "invalid token", errors.New("expired")),
			http.StatusUnauthorized, CodeUnauthorized, "invalid token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			Error(c, tt.bizErr)

			require.Equal(t, tt.wantHTTP, w.Code)
			body := parseBody(t, w)
			require.Equal(t, tt.wantCode, body.Code)
			require.Equal(t, tt.wantMsg, body.Message)
		})
	}
}

// TestError_GenericError 测试 Error 处理普通错误时返回 500 和 CodeInternal。
func TestError_GenericError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	Error(c, errors.New("something broke"))

	require.Equal(t, http.StatusInternalServerError, w.Code)
	body := parseBody(t, w)
	require.Equal(t, CodeInternal, body.Code)
	require.Equal(t, "internal error", body.Message)
}

// TestBizError_Error 测试 BizError.Error() 有无包装错误时的输出。
func TestBizError_Error(t *testing.T) {
	tests := []struct {
		name    string
		bizErr  *BizError
		want    string
	}{
		{
			"without_wrapped",
			NewBizError(http.StatusBadRequest, CodeInvalidArgument, "bad param", nil),
			"bad param",
		},
		{
			"with_wrapped",
			NewBizError(http.StatusInternalServerError, CodeInternal, "db failed", errors.New("connection refused")),
			"db failed: connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.bizErr.Error())
		})
	}
}

// TestBizError_Unwrap 测试 BizError.Unwrap() 返回包装的底层错误。
func TestBizError_Unwrap(t *testing.T) {
	inner := errors.New("root cause")
	bizErr := NewBizError(http.StatusInternalServerError, CodeInternal, "wrap", inner)
	require.ErrorIs(t, bizErr, inner)
	require.Equal(t, inner, bizErr.Unwrap())

	bizErrNil := NewBizError(http.StatusBadRequest, CodeInvalidArgument, "no cause", nil)
	require.Nil(t, bizErrNil.Unwrap())
}

// TestNewBizError 测试 NewBizError 构造函数创建正确的错误。
func TestNewBizError(t *testing.T) {
	cause := errors.New("timeout")
	be := NewBizError(http.StatusGatewayTimeout, CodeServiceConnectFailed, "upstream", cause)

	require.Equal(t, http.StatusGatewayTimeout, be.HTTPStatus)
	require.Equal(t, CodeServiceConnectFailed, be.Code)
	require.Equal(t, "upstream", be.Message)
	require.Equal(t, cause, be.Err)
}
