package apierrors

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
