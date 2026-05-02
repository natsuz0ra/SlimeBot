package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"slimebot/internal/domain"
	"slimebot/internal/logging"

	"github.com/google/uuid"
)

type memoryJobStore interface {
	EnqueueMemoryWriteJob(ctx context.Context, job *domain.MemoryWriteJob) error
	ClaimPendingMemoryWriteJobs(ctx context.Context, limit int) ([]domain.MemoryWriteJob, error)
	MarkMemoryWriteJobDone(ctx context.Context, jobID string) error
	MarkMemoryWriteJobRetry(ctx context.Context, jobID string, nextRetryAt time.Time, errText string) error
	MarkMemoryWriteJobDead(ctx context.Context, jobID string, errText string) error
}

func (s *ChatService) enqueueMemoryWriteJob(ctx context.Context, sessionID, assistantMessageID, messageContent string) error {
	jobStore, ok := s.store.(memoryJobStore)
	if !ok {
		return fmt.Errorf("chat store does not support memory async queue")
	}
	historyDigest, _ := s.buildMemoryHistoryDigest(ctx, sessionID)
	job := &domain.MemoryWriteJob{
		ID:                 uuid.NewString(),
		SessionID:          sessionID,
		AssistantMessageID: assistantMessageID,
		MessageContent:     messageContent,
		HistoryDigest:      historyDigest,
		Status:             "pending",
		Attempt:            0,
		NextRetryAt:        time.Now(),
	}
	return jobStore.EnqueueMemoryWriteJob(ctx, job)
}

func (s *ChatService) maybeEnqueueMemoryAsync(ctx context.Context, sessionID, assistantMessageID, answer string) {
	if !s.memoryAsyncEnabled {
		return
	}
	if strings.TrimSpace(answer) == "" {
		return
	}
	if err := s.enqueueMemoryWriteJob(ctx, sessionID, assistantMessageID, answer); err != nil {
		logging.Warn("memory_async_enqueue_failed", "session", sessionID, "error", err)
		return
	}
	logging.Info("memory_async_enqueued", "session", sessionID)
}

func (s *ChatService) runMemoryAsyncWorker(ctx context.Context) {
	ticker := time.NewTicker(s.memoryAsyncWorkerInterval)
	defer ticker.Stop()
	s.runMemoryAsyncWorkerOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runMemoryAsyncWorkerOnce(ctx)
		}
	}
}

func (s *ChatService) runMemoryAsyncWorkerOnce(ctx context.Context) {
	jobStore, ok := s.store.(memoryJobStore)
	if !ok || s.memory == nil {
		return
	}
	jobs, err := jobStore.ClaimPendingMemoryWriteJobs(ctx, 5)
	if err != nil {
		logging.Warn("memory_async_claim_failed", "error", err)
		return
	}
	for _, job := range jobs {
		payload, skipReason := buildMemoryPayloadFromJob(job)
		if skipReason != "" {
			_ = jobStore.MarkMemoryWriteJobDone(ctx, job.ID)
			logging.Info("memory_async_skip", "job_id", job.ID, "reason", skipReason)
			continue
		}
		s.memory.EnqueueTurnMemory(job.SessionID, job.AssistantMessageID, payload)
		_ = jobStore.MarkMemoryWriteJobDone(ctx, job.ID)
		logging.Info("memory_async_done", "job_id", job.ID, "source", "async_queue")
	}
}

func (s *ChatService) buildMemoryHistoryDigest(ctx context.Context, sessionID string) (string, error) {
	history, err := s.store.ListRecentSessionMessages(ctx, sessionID, 6)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, msg := range history {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(StripContentMarkers(msg.Content))
		if role == "" || content == "" {
			continue
		}
		if len([]rune(content)) > 120 {
			content = string([]rune(content)[:120])
		}
		lines = append(lines, role+": "+content)
	}
	return strings.Join(lines, "\n"), nil
}

func buildMemoryPayloadFromJob(job domain.MemoryWriteJob) (string, string) {
	content := strings.TrimSpace(StripContentMarkers(job.MessageContent))
	if content == "" {
		return "", "empty_message"
	}
	compressed := compressMemoryContent(content)
	if compressed == "" {
		return "", "no_effective_increment"
	}
	out := map[string]string{
		"name":        buildMemoryNameFromContent(compressed),
		"description": buildMemoryDescription(compressed),
		"type":        "project",
		"content":     compressed,
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "", "marshal_failed"
	}
	return string(raw), ""
}

func compressMemoryContent(content string) string {
	content = stripLegacyMemoryBlocks(content)
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "```") || len([]rune(line)) < 6 {
			continue
		}
		kept = append(kept, line)
		if len(kept) >= 4 {
			break
		}
	}
	joined := strings.TrimSpace(strings.Join(kept, " "))
	if len([]rune(joined)) > 320 {
		joined = string([]rune(joined)[:320])
	}
	return joined
}

func stripLegacyMemoryBlocks(content string) string {
	startTag := "<memory>"
	endTag := "</memory>"
	for {
		start := strings.Index(content, startTag)
		if start < 0 {
			return content
		}
		end := strings.Index(content[start+len(startTag):], endTag)
		if end < 0 {
			return content[:start]
		}
		endAbs := start + len(startTag) + end + len(endTag)
		content = content[:start] + content[endAbs:]
	}
}

func buildMemoryNameFromContent(content string) string {
	runes := []rune(content)
	if len(runes) > 24 {
		runes = runes[:24]
	}
	name := strings.TrimSpace(string(runes))
	if name == "" {
		return "Conversation Memory"
	}
	return name
}

func buildMemoryDescription(content string) string {
	runes := []rune(content)
	if len(runes) > 64 {
		runes = runes[:64]
	}
	return strings.TrimSpace(string(runes))
}
