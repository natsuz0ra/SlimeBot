package memory

import (
	"context"
	"log"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

func updateSummaryAsyncImpl(m *MemoryService, modelConfig ModelRuntimeConfig, sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	m.workerMu.Lock()
	state := m.workers[sessionID]
	if state == nil {
		state = &memoryWorkerState{}
		m.workers[sessionID] = state
	}
	if state.running {
		state.pending = true
		m.workerMu.Unlock()
		log.Printf("memory_summary_queued session=%s reason=worker_running", sessionID)
		return
	}
	state.running = true
	m.workerMu.Unlock()

	go runSummaryWorkerImpl(m, modelConfig, sessionID)
}

func runSummaryWorkerImpl(m *MemoryService, modelConfig ModelRuntimeConfig, sessionID string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("memory_summary_panic session=%s recovered=%v", sessionID, recovered)
		}
		m.workerMu.Lock()
		delete(m.workers, sessionID)
		m.workerMu.Unlock()
	}()

	for {
		runSummaryOnceImpl(m, modelConfig, sessionID)

		m.workerMu.Lock()
		state, ok := m.workers[sessionID]
		if !ok {
			m.workerMu.Unlock()
			return
		}
		if state.pending {
			state.pending = false
			m.workerMu.Unlock()
			log.Printf("memory_summary_worker_continue session=%s reason=pending_trigger", sessionID)
			continue
		}
		m.workerMu.Unlock()
		return
	}
}

func runSummaryOnceImpl(m *MemoryService, modelConfig ModelRuntimeConfig, sessionID string) {
	startAt := time.Now()

	ctx := context.Background()
	totalMessages, err := m.store.CountSessionMessages(sessionID)
	if err != nil {
		log.Printf("memory_summary_skip session=%s reason=count_failed err=%v", sessionID, err)
		return
	}
	if totalMessages == 0 {
		return
	}

	recent, err := m.store.ListRecentSessionMessages(sessionID, constants.MemorySummaryRecentMessageSize)
	if err != nil {
		log.Printf("memory_summary_skip session=%s reason=recent_failed err=%v", sessionID, err)
		return
	}
	if len(recent) == 0 {
		return
	}

	existing, err := m.store.GetSessionMemory(sessionID)
	if err != nil {
		log.Printf("memory_summary_skip session=%s reason=get_existing_failed err=%v", sessionID, err)
		return
	}

	oldSummary := ""
	if existing != nil {
		oldSummary = existing.Summary
	}

	mergedSummary, attempts, summaryCost, err := m.MergeSummary(ctx, modelConfig, oldSummary, recent)
	if err != nil {
		log.Printf(
			"memory_summary_skip session=%s reason=merge_failed attempts=%d cost_ms=%d timeout_ms=%d err_class=%s err=%v",
			sessionID,
			attempts,
			summaryCost.Milliseconds(),
			constants.MemorySummaryTimeout.Milliseconds(),
			classifyMemoryError(err),
			err,
		)
		return
	}
	keywords := m.TokenizeKeywords(mergedSummary + "\n" + flattenMessages(recent))
	updated, err := m.store.UpsertSessionMemoryIfNewer(domain.SessionMemoryUpsertInput{
		SessionID:          sessionID,
		Summary:            mergedSummary,
		Keywords:           keywords,
		SourceMessageCount: int(totalMessages),
	})
	if err != nil {
		log.Printf("memory_summary_skip session=%s reason=upsert_failed err=%v", sessionID, err)
		return
	}
	if !updated {
		log.Printf("memory_summary_skip session=%s reason=stale_write source_message_count=%d", sessionID, totalMessages)
		return
	}

	if m.embedding != nil && m.vectorStore != nil {
		if err := m.upsertSessionMemoryVector(ctx, sessionID, mergedSummary, keywords, int(totalMessages)); err != nil {
			// 详细失败日志已在 upsertSessionMemoryVector 内记录，这里避免重复打印。
		}
	}

	log.Printf(
		"memory_summary_updated session=%s total_messages=%d keywords=%d attempts=%d summary_cost_ms=%d timeout_ms=%d total_cost_ms=%d",
		sessionID,
		totalMessages,
		len(keywords),
		attempts,
		summaryCost.Milliseconds(),
		constants.MemorySummaryTimeout.Milliseconds(),
		time.Since(startAt).Milliseconds(),
	)
}
