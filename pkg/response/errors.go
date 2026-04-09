package response

import "fmt"

const (
	// CodeSuccess 表示成功
	CodeSuccess = 0

	// CodeInvalidArgument 表示参数错误
	CodeInvalidArgument = 1001
	// CodeNotFound 表示资源不存在
	CodeNotFound = 1002
	// CodeConflict 表示资源冲突
	CodeConflict = 1003

	// CodeUnauthorized 表示未认证
	CodeUnauthorized = 2001
	// CodeTokenExpired 表示令牌过期
	CodeTokenExpired = 2002
	// CodeForbidden 表示权限不足
	CodeForbidden = 2003

	// CodeServiceConnectFailed 表示服务连接失败
	CodeServiceConnectFailed = 3001
	// CodeToolInvokeFailed 表示工具调用失败
	CodeToolInvokeFailed = 3002
	// CodeTooManyRequests 表示并发或限流命中
	CodeTooManyRequests = 3003

	// CodeInternal 表示系统错误
	CodeInternal = 5001
)

// BizError 定义业务错误
type BizError struct {
	HTTPStatus int
	Code       int
	Message    string
	Err        error
}

// Error 返回错误文本
func (e *BizError) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

// Unwrap 返回底层错误
func (e *BizError) Unwrap() error {
	return e.Err
}

// NewBizError 创建业务错误
func NewBizError(httpStatus, code int, message string, err error) *BizError {
	return &BizError{HTTPStatus: httpStatus, Code: code, Message: message, Err: err}
}
