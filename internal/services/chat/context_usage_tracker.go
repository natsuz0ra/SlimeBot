package chat

import (
	"sync"
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

func (t *contextUsageTracker) setUsedTokens(usedTokens int) error {
	if usedTokens < 0 {
		usedTokens = 0
	}
	t.mu.Lock()
	t.usage.UsedTokens = usedTokens
	t.usage.Source = "provider-reported"
	usage := normalizeContextUsagePercentages(t.usage)
	t.usage = usage
	t.mu.Unlock()
	return t.emit(usage)
}

func (t *contextUsageTracker) emit(usage ContextUsage) error {
	if t.onUsage == nil {
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
