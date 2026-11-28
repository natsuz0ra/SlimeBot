package apperrors

import "errors"

// Shared application errors let services and HTTP adapters agree on semantics
// without exposing implementation-specific failures to clients.
var (
	// ErrNotFound means the requested resource does not exist.
	ErrNotFound = errors.New("not found")
	// ErrInvalidInput means the input parameters are invalid.
	ErrInvalidInput = errors.New("invalid input")
	// ErrConflict means the request conflicts with current resource state.
	ErrConflict = errors.New("conflict")
	// ErrUnauthorized means authentication is absent or invalid.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden means the authenticated caller lacks permission.
	ErrForbidden = errors.New("forbidden")
	// ErrUnavailable means a required dependency is temporarily unavailable.
	ErrUnavailable = errors.New("unavailable")
)
