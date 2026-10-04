package llm

import (
	"errors"
	"fmt"
	"slimebot/internal/constants"
	"strings"
)

// ResolveRequestConfig is used by both budgeting and wire adapters.
func ResolveRequestConfig(c ModelRuntimeConfig) (ModelRuntimeConfig, error) {
	if c.ContextSize <= 0 {
		c.ContextSize = constants.DefaultContextSize
	}
	if c.MaxOutputTokens <= 0 {
		c.MaxOutputTokens = max(128, min(4096, c.ContextSize/8))
	}
	if c.Provider == ProviderAnthropic {
		c.MaxOutputTokens = max(c.MaxOutputTokens, ThinkingBudgetTokens(c.ThinkingLevel)+1)
	}
	if c.MaxOutputTokens >= c.ContextSize {
		return c, fmt.Errorf("输出/思考预算 %d 已达到上下文窗口 %d，请调整模型配置", c.MaxOutputTokens, c.ContextSize)
	}
	return c, nil
}

// InputContextTokens excludes output and counts provider caches exactly once.
func (u TokenUsage) InputContextTokens(provider string) int {
	if provider == ProviderAnthropic {
		return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	}
	return u.InputTokens
}

type ContextWindowError struct{ Err error }

func (e *ContextWindowError) Error() string  { return e.Err.Error() }
func (e *ContextWindowError) Unwrap() error  { return e.Err }
func IsContextWindowExceeded(err error) bool { var e *ContextWindowError; return errors.As(err, &e) }

// ClassifyContextError requires explicit context-window semantics, never a generic 400.
func ClassifyContextError(err error) error {
	if err == nil {
		return nil
	}
	text := strings.ToLower(err.Error())
	for _, code := range []string{"context_length_exceeded", "context_window_exceeded", "maximum context length", "prompt is too long", "exceeds the context window"} {
		if strings.Contains(text, code) {
			return &ContextWindowError{Err: err}
		}
	}
	return err
}
