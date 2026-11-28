package apierrors

import (
	"context"
	"errors"
	"net/http"

	"slimebot/internal/apperrors"
	"slimebot/internal/logging"
)

// Stable machine-readable error codes returned alongside the legacy error text.
const (
	CodeInvalidInput  = "invalid_input"
	CodeNotFound      = "not_found"
	CodeConflict      = "conflict"
	CodeUnauthorized  = "unauthorized"
	CodeForbidden     = "forbidden"
	CodeTimeout       = "timeout"
	CodeUnavailable   = "unavailable"
	CodeInternalError = "internal_error"
)

// MapError maps known application errors to a stable public contract. Unknown
// failures are logged server-side and deliberately reduced to a generic body.
func MapError(err error) (status int, body APIError) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidInput):
		return http.StatusBadRequest, APIError{Code: CodeInvalidInput, Message: "invalid input"}
	case errors.Is(err, apperrors.ErrNotFound):
		return http.StatusNotFound, APIError{Code: CodeNotFound, Message: "not found"}
	case errors.Is(err, apperrors.ErrConflict):
		return http.StatusConflict, APIError{Code: CodeConflict, Message: "conflict"}
	case errors.Is(err, apperrors.ErrUnauthorized):
		return http.StatusUnauthorized, APIError{Code: CodeUnauthorized, Message: "unauthorized"}
	case errors.Is(err, apperrors.ErrForbidden):
		return http.StatusForbidden, APIError{Code: CodeForbidden, Message: "forbidden"}
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, APIError{Code: CodeTimeout, Message: "request timeout"}
	case errors.Is(err, apperrors.ErrUnavailable):
		return http.StatusServiceUnavailable, APIError{Code: CodeUnavailable, Message: "service unavailable"}
	default:
		logging.Error("api_internal_error", "err", err)
		return http.StatusInternalServerError, APIError{Code: CodeInternalError, Message: "internal server error"}
	}
}

// CodeForStatus adds the same stable code to errors constructed directly by
// controllers while preserving their existing human-readable messages.
func CodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return CodeInvalidInput
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusConflict:
		return CodeConflict
	case http.StatusUnauthorized:
		return CodeUnauthorized
	case http.StatusForbidden:
		return CodeForbidden
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return CodeTimeout
	case http.StatusServiceUnavailable, http.StatusBadGateway:
		return CodeUnavailable
	case http.StatusInternalServerError:
		return CodeInternalError
	default:
		return ""
	}
}
