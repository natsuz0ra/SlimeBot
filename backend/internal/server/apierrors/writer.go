package apierrors

import (
	"encoding/json"
	"net/http"
)

// WriteJSONError 直接写出统一错误对象（包含至少 `{"error": "..."} `）。
func WriteJSONError(w http.ResponseWriter, status int, apiErr APIError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiErr)
}
