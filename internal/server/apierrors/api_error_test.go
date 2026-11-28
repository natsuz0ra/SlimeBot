package apierrors

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"slimebot/internal/apperrors"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestMapErrorKnownClassifications(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		code    string
		message string
	}{
		{name: "invalid input", err: fmt.Errorf("create: %w", apperrors.ErrInvalidInput), status: http.StatusBadRequest, code: CodeInvalidInput, message: "invalid input"},
		{name: "not found", err: fmt.Errorf("load: %w", apperrors.ErrNotFound), status: http.StatusNotFound, code: CodeNotFound, message: "not found"},
		{name: "conflict", err: apperrors.ErrConflict, status: http.StatusConflict, code: CodeConflict, message: "conflict"},
		{name: "unauthorized", err: apperrors.ErrUnauthorized, status: http.StatusUnauthorized, code: CodeUnauthorized, message: "unauthorized"},
		{name: "forbidden", err: apperrors.ErrForbidden, status: http.StatusForbidden, code: CodeForbidden, message: "forbidden"},
		{name: "deadline", err: fmt.Errorf("stream: %w", context.DeadlineExceeded), status: http.StatusGatewayTimeout, code: CodeTimeout, message: "request timeout"},
		{name: "unavailable", err: apperrors.ErrUnavailable, status: http.StatusServiceUnavailable, code: CodeUnavailable, message: "service unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := MapError(tt.err)
			if status != tt.status || body.Code != tt.code || body.Message != tt.message {
				t.Fatalf("MapError() = (%d, %+v), want (%d, %s, %s)", status, body, tt.status, tt.code, tt.message)
			}
		})
	}
}

func TestMapErrorSanitizesAndLogsUnknownInternalError(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	previous := zap.L()
	zap.ReplaceGlobals(zap.New(core))
	t.Cleanup(func() { zap.ReplaceGlobals(previous) })

	secret := "database password=hunter2 stack=private"
	status, body := MapError(errors.New(secret))
	if status != http.StatusInternalServerError || body.Code != CodeInternalError || body.Message != "internal server error" {
		t.Fatalf("unexpected internal mapping: status=%d body=%+v", status, body)
	}
	if strings.Contains(body.Message, "hunter2") || body.Details != nil {
		t.Fatalf("internal error leaked implementation details: %+v", body)
	}
	entries := logs.FilterMessage("api_internal_error").All()
	if len(entries) != 1 {
		t.Fatalf("internal error log count = %d, want 1", len(entries))
	}
	if got := entries[0].ContextMap()["err"]; got != secret {
		t.Fatalf("server log error = %v, want original error", got)
	}
}

func TestCodeForStatusUsesStableCodes(t *testing.T) {
	for status, want := range map[int]string{
		http.StatusBadRequest:          CodeInvalidInput,
		http.StatusNotFound:            CodeNotFound,
		http.StatusConflict:            CodeConflict,
		http.StatusUnauthorized:        CodeUnauthorized,
		http.StatusForbidden:           CodeForbidden,
		http.StatusRequestTimeout:      CodeTimeout,
		http.StatusGatewayTimeout:      CodeTimeout,
		http.StatusBadGateway:          CodeUnavailable,
		http.StatusServiceUnavailable:  CodeUnavailable,
		http.StatusInternalServerError: CodeInternalError,
	} {
		if got := CodeForStatus(status); got != want {
			t.Errorf("CodeForStatus(%d) = %q, want %q", status, got, want)
		}
	}
}
