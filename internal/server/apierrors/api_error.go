package apierrors

import "net/http"

// APIError 统一的 API 错误对象。
// 兼容性要求：至少包含 `{"error": "..."}`
type APIError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"error"`
	Details any    `json:"details,omitempty"`
}

func (e APIError) Error() string {
	return e.Message
}

// StatusCode 根据 error code 映射 HTTP 状态码。
// 未知 code 默认返回 500。
func (e APIError) StatusCode() int {
	switch e.Code {
	case "bad_request", "invalid_argument":
		return http.StatusBadRequest
	case "unauthorized":
		return http.StatusUnauthorized
	case "forbidden":
		return http.StatusForbidden
	case "not_found":
		return http.StatusNotFound
	case "conflict":
		return http.StatusConflict
	case "internal_error":
		return http.StatusInternalServerError
	default:
		// code 为空/未知时，仍以 message 写回，调用方可覆盖 status。
		return http.StatusInternalServerError
	}
}

func New(code string, message string, details any) APIError {
	return APIError{
		Code:    code,
		Message: message,
		Details: details,
	}
}

func BadRequest(message string) APIError {
	return New("bad_request", message, nil)
}

func Unauthorized(message string) APIError {
	return New("unauthorized", message, nil)
}

func Forbidden(message string) APIError {
	return New("forbidden", message, nil)
}

func NotFound(message string) APIError {
	return New("not_found", message, nil)
}

func Conflict(message string) APIError {
	return New("conflict", message, nil)
}

func Internal(message string) APIError {
	return New("internal_error", message, nil)
}
