package memory

import (
	"context"
	"errors"
	"net"
	"strings"
)

// isRetryableMemoryError 判定 memory 阶段是否可进行一次重试（超时/网络抖动）。
func isRetryableMemoryError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	text := strings.ToLower(err.Error())
	return strings.Contains(text, "deadline exceeded") ||
		strings.Contains(text, "timeout") ||
		strings.Contains(text, "i/o timeout") ||
		strings.Contains(text, "connection reset") ||
		strings.Contains(text, "broken pipe") ||
		strings.Contains(text, "eof")
}

// classifyMemoryError 将错误归类为日志标签，便于观测与排障。
func classifyMemoryError(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "network_timeout"
		}
		return "network_error"
	}

	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "deadline exceeded"), strings.Contains(text, "timeout"), strings.Contains(text, "i/o timeout"):
		return "deadline_exceeded"
	case strings.Contains(text, "connection reset"), strings.Contains(text, "broken pipe"), strings.Contains(text, "eof"):
		return "network_error"
	default:
		return "unknown"
	}
}
