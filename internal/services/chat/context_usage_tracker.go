package chat

import (
	"sync"

	llmsvc "slimebot/internal/services/llm"
)

type contextUsageTracker struct {
	mu      sync.Mutex
	usage   ContextUsage
	onUsage func(ContextUsage) error
}

func newContextUsageTracker(initial ContextUsage, onUsage func(ContextUsage) error) *contextUsageTracker {
	return &contextUsageTracker{
		usage:   initial,
		onUsage: onUsage,
	}
}

func (t *contextUsageTracker) calibrateProviderUsage(usage llmsvc.TokenUsage) error {
	if usage.IsZero() {
		return nil
	}
	return t.setUsedTokens(usage.ContextWindowTokens())
}

func (t *contextUsageTracker) setUsedTokens(usedTokens int) error {
	if usedTokens < 0 {
		usedTokens = 0
	}
	t.mu.Lock()
	t.usage.UsedTokens = usedTokens
	usage := normalizeContextUsagePercentages(t.usage)
	t.usage = usage
	t.mu.Unlock()
	return t.emit(usage)
}

func (t *contextUsageTracker) emit(usage ContextUsage) error {
	if t == nil || t.onUsage == nil {
		return nil
	}
	return t.onUsage(usage)
}

func normalizeContextUsagePercentages(usage ContextUsage) ContextUsage {
	usedPercent := 0
	if usage.TotalTokens > 0 {
		usedPercent = int(float64(usage.UsedTokens)*100/float64(usage.TotalTokens) + 0.5)
	}
	if usedPercent < 0 {
		usedPercent = 0
	}
	if usedPercent > 100 {
		usedPercent = 100
	}
	usage.UsedPercent = usedPercent
	usage.AvailablePercent = 100 - usedPercent
	return usage
}
