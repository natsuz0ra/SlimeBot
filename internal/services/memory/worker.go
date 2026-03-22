package memory

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"slimebot/internal/domain"
)

type memoryWorkerState struct {
	running           bool
	pending           bool
	lastRawSummary    string
	pendingRawSummary string
}

// updateSummaryAsyncImpl 为单会话启动摘要 worker；若已运行则排队最新摘要。
func updateSummaryAsyncImpl(m *MemoryService, sessionID string, rawSummary string) {
	sessionID = strings.TrimSpace(sessionID)
	rawSummary = strings.TrimSpace(rawSummary)
	if sessionID == "" || rawSummary == "" {
		return
	}

	m.workerMu.Lock()
	state := m.workers[sessionID]
	if state == nil {
		state = &memoryWorkerState{}
		m.workers[sessionID] = state
	}
	state.lastRawSummary = rawSummary
	if state.running {
		// worker 运行中只保留最新待处理摘要。
		state.pending = true
		state.pendingRawSummary = rawSummary
		m.workerMu.Unlock()
		slog.Info("memory_summary_queued", "session", sessionID, "reason", "worker_running")
		return
	}
	state.running = true
	m.workerMu.Unlock()

	m.workerWg.Add(1)
	go func() {
		defer m.workerWg.Done()
		runSummaryWorkerImpl(m, sessionID)
	}()
}

// runSummaryWorkerImpl 单会话 worker 循环：处理摘要并在 pending 时继续。
func runSummaryWorkerImpl(m *MemoryService, sessionID string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("memory_summary_panic", "session", sessionID, "recovered", recovered)
		}
		m.workerMu.Lock()
		delete(m.workers, sessionID)
		m.workerMu.Unlock()
	}()

	for {
		select {
		case <-m.workerCtx.Done():
			return
		default:
		}

		var summary string
		m.workerMu.Lock()
		state := m.workers[sessionID]
		if state == nil {
			m.workerMu.Unlock()
			return
		}
		summary = state.lastRawSummary
		m.workerMu.Unlock()

		runSummaryOnceImpl(m, sessionID, summary)

		m.workerMu.Lock()
		state = m.workers[sessionID]
		if state == nil {
			m.workerMu.Unlock()
			return
		}
		if state.pending {
			// 有新摘要待处理则继续下一轮。
			state.pending = false
			state.lastRawSummary = state.pendingRawSummary
			state.pendingRawSummary = ""
			m.workerMu.Unlock()
			slog.Info("memory_summary_worker_continue", "session", sessionID, "reason", "pending_trigger")
			continue
		}
		m.workerMu.Unlock()
		return
	}
}

// runSummaryOnceImpl 解析摘要 ops 并执行增删改，同步向量库。
func runSummaryOnceImpl(m *MemoryService, sessionID string, rawSummary string) {
	startAt := time.Now()
	ctx := m.workerCtx
	if ctx == nil {
		ctx = context.Background()
	}

	ops, err := parseMemoryOps(rawSummary)
	if err != nil {
		// 解析失败时降级为单条 create。
		ops = parseMemoryOpsOrFallback(rawSummary)
	}
	if len(ops) == 0 {
		return
	}

	totalMessages, err := m.store.CountSessionMessages(sessionID)
	if err != nil {
		slog.Warn("memory_summary_skip", "session", sessionID, "reason", "count_failed", "err", err)
		return
	}

	for _, op := range ops {
		switch op.Action {
		case "create":
			kw := m.TokenizeKeywords(op.Content)
			created, cerr := m.store.CreateSessionMemory(domain.SessionMemoryCreateInput{
				SessionID:          sessionID,
				Summary:            op.Content,
				Keywords:           kw,
				SourceMessageCount: int(totalMessages),
			})
			if cerr != nil {
				slog.Warn("memory_create_failed", "session", sessionID, "err", cerr)
				continue
			}
			if m.embedding != nil && m.vectorStore != nil && created != nil {
				_ = upsertMemoryVector(m, ctx, created.ID, sessionID, created.Summary, kw, int(totalMessages))
			}
		case "update":
			kw := m.TokenizeKeywords(op.Content)
			if err := m.store.UpdateSessionMemoryContent(op.ID, sessionID, op.Content, kw, int(totalMessages)); err != nil {
				slog.Warn("memory_update_failed", "session", sessionID, "id", op.ID, "err", err)
				continue
			}
			if m.embedding != nil && m.vectorStore != nil {
				_ = upsertMemoryVector(m, ctx, op.ID, sessionID, op.Content, kw, int(totalMessages))
			}
		case "delete":
			if err := m.store.SoftDeleteSessionMemory(op.ID, sessionID); err != nil {
				slog.Warn("memory_delete_failed", "session", sessionID, "id", op.ID, "err", err)
				continue
			}
			if m.vectorStore != nil {
				_ = m.vectorStore.DeleteMemoryVector(ctx, op.ID)
			}
		}
	}

	slog.Info("memory_summary_updated",
		"session", sessionID,
		"total_messages", totalMessages,
		"ops", len(ops),
		"total_cost_ms", time.Since(startAt).Milliseconds(),
	)
}
