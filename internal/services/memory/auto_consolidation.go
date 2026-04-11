package memory

import (
	"fmt"
	"time"
)

// ConfigureAutoConsolidation updates the automatic consolidation gates.
func (m *MemoryService) ConfigureAutoConsolidation(enabled bool, minInterval time.Duration, minEntries int) {
	if m == nil {
		return
	}
	if minEntries <= 0 {
		minEntries = 1
	}

	m.autoConfigMu.Lock()
	defer m.autoConfigMu.Unlock()
	m.autoConsolidationEnabled = enabled
	m.autoConsolidationMinInterval = minInterval
	m.autoConsolidationMinEntries = minEntries
}

// SetConsolidateHookForTest injects a hook before consolidation runs.
func (m *MemoryService) SetConsolidateHookForTest(hook func()) {
	if m == nil {
		return
	}
	m.autoConfigMu.Lock()
	defer m.autoConfigMu.Unlock()
	m.consolidateHookForTest = hook
}

// TryAutoConsolidate runs one consolidation pass when gates allow it.
// Returns whether a run happened, plus merge/delete counts.
func (m *MemoryService) TryAutoConsolidate(trigger string) (bool, int, int, error) {
	if m == nil || m.store == nil {
		return false, 0, 0, nil
	}
	if !m.autoConsolidationRunning.CompareAndSwap(false, true) {
		return false, 0, 0, nil
	}
	defer m.autoConsolidationRunning.Store(false)

	m.autoConfigMu.RLock()
	enabled := m.autoConsolidationEnabled
	minInterval := m.autoConsolidationMinInterval
	minEntries := m.autoConsolidationMinEntries
	lastRun := m.lastAutoConsolidatedAt
	hook := m.consolidateHookForTest
	m.autoConfigMu.RUnlock()

	if !enabled {
		return false, 0, 0, nil
	}

	entries, err := m.store.List()
	if err != nil {
		return false, 0, 0, fmt.Errorf("list memories before auto consolidate: %w", err)
	}
	if len(entries) < minEntries {
		return false, 0, 0, nil
	}
	if minInterval > 0 && !lastRun.IsZero() && time.Since(lastRun) < minInterval {
		return false, 0, 0, nil
	}

	if hook != nil {
		hook()
	}

	merged, deleted, err := m.Consolidate()
	if err != nil {
		return true, 0, 0, err
	}

	m.autoConfigMu.Lock()
	m.lastAutoConsolidatedAt = time.Now()
	m.autoConfigMu.Unlock()

	_ = trigger
	return true, merged, deleted, nil
}
