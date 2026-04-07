package memory

import (
	"context"
	"slimebot/internal/logging"
	"strings"
	"time"

	"slimebot/internal/domain"
)

func enqueueTurnMemoryImpl(m *MemoryService, sessionID, assistantMessageID, rawMemoryPayload string) {
	sessionID = strings.TrimSpace(sessionID)
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	rawMemoryPayload = strings.TrimSpace(rawMemoryPayload)
	if sessionID == "" || assistantMessageID == "" || rawMemoryPayload == "" {
		return
	}

	job := queuedTurnMemory{assistantMessageID: assistantMessageID, rawPayload: rawMemoryPayload}

	m.workerMu.Lock()
	state := m.workers[sessionID]
	if state == nil {
		state = &memoryWorkerState{}
		m.workers[sessionID] = state
	}
	state.pending = append(state.pending, job)
	if state.running {
		m.workerMu.Unlock()
		enqueueLog(sessionID, rawMemoryPayload)
		return
	}
	state.running = true
	m.workerMu.Unlock()

	enqueueLog(sessionID, rawMemoryPayload)
	m.workerWg.Add(1)
	go func() {
		defer m.workerWg.Done()
		runTurnMemoryWorker(m, sessionID)
	}()
}

func runTurnMemoryWorker(m *MemoryService, sessionID string) {
	defer func() {
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
		job, ok := dequeueTurnMemory(m, sessionID)
		if !ok {
			return
		}
		processTurnMemory(m, sessionID, job)
	}
}

func dequeueTurnMemory(m *MemoryService, sessionID string) (queuedTurnMemory, bool) {
	m.workerMu.Lock()
	defer m.workerMu.Unlock()
	state := m.workers[sessionID]
	if state == nil || len(state.pending) == 0 {
		return queuedTurnMemory{}, false
	}
	job := state.pending[0]
	state.pending = state.pending[1:]
	return job, true
}

func processTurnMemory(m *MemoryService, sessionID string, job queuedTurnMemory) {
	ctx := m.workerCtx
	if ctx == nil {
		ctx = context.Background()
	}

	payload, err := parseTurnMemoryPayload(job.rawPayload)
	if err != nil {
		logging.Warn("memory_payload_parse_failed", "session", sessionID, "err", err)
		return
	}
	msg, err := m.store.GetMessageByID(ctx, job.assistantMessageID)
	if err != nil || msg == nil {
		logging.Warn("memory_turn_message_missing", "session", sessionID, "assistant_message_id", job.assistantMessageID, "err", err)
		return
	}

	startSeq := msg.Seq
	if startSeq > 1 {
		startSeq--
	}
	for _, item := range payload.Sticky {
		applyStickyPayload(ctx, m, sessionID, item, startSeq, msg.Seq, msg.CreatedAt)
	}

	topicKey := deriveTopicKey(payload.TopicHint, payload.Keywords)
	if topicKey == "" {
		topicKey = fallbackTopicKey(payload.TurnSummary, payload.Keywords)
	}
	if strings.TrimSpace(payload.TurnSummary) == "" {
		payload.TurnSummary = strings.TrimSpace(payload.TopicHint)
	}
	if strings.TrimSpace(payload.TurnSummary) == "" {
		return
	}
	if err := m.applyEpisodePayload(ctx, sessionID, msg, payload, startSeq, topicKey); err != nil {
		logging.Warn("memory_episode_apply_failed", "session", sessionID, "topic", topicKey, "err", err)
	}
}

func applyStickyPayload(ctx context.Context, m *MemoryService, sessionID string, item stickyPayload, startSeq, endSeq int64, seenAt time.Time) {
	switch item.Action {
	case "delete":
		_ = m.store.DeleteStickyMemory(ctx, sessionID, item.Kind, item.Key)
	case "upsert":
		_, _ = m.store.UpsertStickyMemory(domain.StickyMemoryUpsertInput{
			SessionID:      sessionID,
			Kind:           item.Kind,
			Key:            item.Key,
			Value:          item.Value,
			Summary:        item.Summary,
			Confidence:     item.Confidence,
			SourceStartSeq: startSeq,
			SourceEndSeq:   endSeq,
			LastSeenAt:     seenAt,
		})
	}
}

func (m *MemoryService) applyEpisodePayload(ctx context.Context, sessionID string, msg *domain.Message, payload turnMemoryPayload, startSeq int64, topicKey string) error {
	now := msg.CreatedAt
	openEpisode, err := m.store.GetOpenEpisodeMemory(ctx, sessionID)
	if err != nil {
		return err
	}

	if openEpisode != nil {
		score := continuityScore(openEpisode, payload, now)
		shouldContinue := score >= 0.60 || (score >= 0.45 && score < 0.60 && safeTimeGap(now, openEpisode.LastActiveAt) <= splitSoftWindow && keywordOverlap(openEpisode, payload.Keywords) > 0)
		if shouldContinue {
			return m.updateEpisode(ctx, openEpisode, payload, startSeq, msg.Seq, now, domain.EpisodeMemoryStateOpen, topicKey)
		}
		if err := m.store.UpdateEpisodeMemory(domain.EpisodeMemoryUpdateInput{
			ID:             openEpisode.ID,
			SessionID:      sessionID,
			TopicKey:       openEpisode.TopicKey,
			Title:          openEpisode.Title,
			Summary:        openEpisode.Summary,
			Keywords:       mergeKeywords(openEpisode.KeywordsJSON, nil),
			State:          domain.EpisodeMemoryStateClosed,
			SourceStartSeq: openEpisode.SourceStartSeq,
			SourceEndSeq:   openEpisode.SourceEndSeq,
			TurnCount:      openEpisode.TurnCount,
			LastActiveAt:   openEpisode.LastActiveAt,
		}); err != nil {
			return err
		}
	}

	reopen, err := m.store.GetLatestClosedEpisodeByTopicKey(ctx, sessionID, topicKey)
	if err != nil {
		return err
	}
	if reopen != nil && safeTimeGap(now, reopen.LastActiveAt) <= reopenTopicWindow {
		return m.updateEpisode(ctx, reopen, payload, reopen.SourceStartSeq, msg.Seq, now, domain.EpisodeMemoryStateOpen, topicKey)
	}

	return m.createEpisode(ctx, sessionID, payload, startSeq, msg.Seq, now, topicKey)
}

func (m *MemoryService) createEpisode(ctx context.Context, sessionID string, payload turnMemoryPayload, startSeq, endSeq int64, now time.Time, topicKey string) error {
	item, err := m.store.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      sessionID,
		TopicKey:       topicKey,
		Title:          pickEpisodeTitle(payload),
		Summary:        strings.TrimSpace(payload.TurnSummary),
		Keywords:       payload.Keywords,
		State:          domain.EpisodeMemoryStateOpen,
		SourceStartSeq: startSeq,
		SourceEndSeq:   endSeq,
		TurnCount:      1,
		LastActiveAt:   now,
	})
	if err != nil {
		return err
	}
	if m.embedding != nil && m.vectorStore != nil {
		_ = upsertEpisodeVector(m, ctx, item)
	}
	return nil
}

func (m *MemoryService) updateEpisode(ctx context.Context, episode *domain.EpisodeMemory, payload turnMemoryPayload, startSeq, endSeq int64, now time.Time, state, topicKey string) error {
	keywords := mergeKeywords(episode.KeywordsJSON, payload.Keywords)
	if err := m.store.UpdateEpisodeMemory(domain.EpisodeMemoryUpdateInput{
		ID:             episode.ID,
		SessionID:      episode.SessionID,
		TopicKey:       chooseTopicKey(topicKey, episode.TopicKey),
		Title:          chooseTitle(payload, episode.Title),
		Summary:        mergeSummary(episode.Summary, payload.TurnSummary),
		Keywords:       keywords,
		State:          state,
		SourceStartSeq: minSeq(episode.SourceStartSeq, startSeq),
		SourceEndSeq:   endSeq,
		TurnCount:      episode.TurnCount + 1,
		LastActiveAt:   now,
	}); err != nil {
		return err
	}
	if m.embedding != nil && m.vectorStore != nil {
		updated := *episode
		updated.TopicKey = chooseTopicKey(topicKey, episode.TopicKey)
		updated.Title = chooseTitle(payload, episode.Title)
		updated.Summary = mergeSummary(episode.Summary, payload.TurnSummary)
		updated.KeywordsJSON = encodeKeywordsForService(keywords)
		updated.State = state
		updated.SourceStartSeq = minSeq(episode.SourceStartSeq, startSeq)
		updated.SourceEndSeq = endSeq
		updated.TurnCount = episode.TurnCount + 1
		updated.LastActiveAt = now
		_ = upsertEpisodeVector(m, ctx, &updated)
	}
	return nil
}

func encodeKeywordsForService(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	payload := `["` + strings.Join(items, `","`) + `"]`
	return payload
}

func continuityScore(open *domain.EpisodeMemory, payload turnMemoryPayload, now time.Time) float64 {
	textKeywords := mergeKeywords(open.KeywordsJSON, nil)
	overlap := float64(keywordOverlap(open, payload.Keywords))
	overlapScore := 0.0
	if len(textKeywords) > 0 {
		overlapScore = overlap / float64(len(textKeywords))
	}
	topicScore := 0.0
	if strings.TrimSpace(payload.TopicHint) != "" && strings.EqualFold(strings.TrimSpace(payload.TopicHint), strings.TrimSpace(open.TopicKey)) {
		topicScore = 1
	}
	timeScore := 1.0
	if gap := safeTimeGap(now, open.LastActiveAt); gap > splitHardWindow {
		timeScore = 0
	} else if gap > splitSoftWindow {
		timeScore = 0.5
	}
	semanticScore := similarityFromKeywords(textKeywords, payload.Keywords)
	return 0.55*semanticScore + 0.20*overlapScore + 0.15*topicScore + 0.10*timeScore
}

func similarityFromKeywords(left []string, right []string) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(left))
	for _, item := range left {
		seen[item] = struct{}{}
	}
	matched := 0
	for _, item := range right {
		if _, ok := seen[item]; ok {
			matched++
		}
	}
	denom := len(left)
	if len(right) > denom {
		denom = len(right)
	}
	return float64(matched) / float64(denom)
}

func keywordOverlap(open *domain.EpisodeMemory, incoming []string) int {
	existing := mergeKeywords(open.KeywordsJSON, nil)
	seen := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		seen[item] = struct{}{}
	}
	matched := 0
	for _, item := range incoming {
		if _, ok := seen[item]; ok {
			matched++
		}
	}
	return matched
}

func chooseTitle(payload turnMemoryPayload, fallback string) string {
	if title := pickEpisodeTitle(payload); title != "" {
		return title
	}
	return strings.TrimSpace(fallback)
}

func pickEpisodeTitle(payload turnMemoryPayload) string {
	if strings.TrimSpace(payload.TopicHint) != "" {
		return strings.TrimSpace(payload.TopicHint)
	}
	if len(payload.Keywords) > 0 {
		return payload.Keywords[0]
	}
	return strings.TrimSpace(payload.TurnSummary)
}

func chooseTopicKey(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return strings.TrimSpace(primary)
	}
	return strings.TrimSpace(fallback)
}

func minSeq(left, right int64) int64 {
	if left == 0 {
		return right
	}
	if right == 0 || left < right {
		return left
	}
	return right
}
