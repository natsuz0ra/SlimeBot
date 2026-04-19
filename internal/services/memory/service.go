package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slimebot/internal/logging"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

// MemoryService wraps FileMemoryStore and exposes a single interface for callers.
// Kept compatible with chat/agent services.
type MemoryService struct {
	store *FileMemoryStore

	autoConfigMu                 sync.RWMutex
	autoConsolidationEnabled     bool
	autoConsolidationMinInterval time.Duration
	autoConsolidationMinEntries  int
	lastAutoConsolidatedAt       time.Time
	consolidateHookForTest       func()
	autoConsolidationRunning     atomic.Bool
}

// MemorySearchHit is one hit in a memory search result.
type MemorySearchHit struct {
	Kind      string
	ID        string
	Title     string
	Summary   string
	Score     float64
	CreatedAt time.Time
}

// MemoryQueryResult is the full memory search result.
type MemoryQueryResult struct {
	Query  string
	Hits   []MemorySearchHit
	Output string
}

// NewMemoryService creates the memory service. baseDir is usually ~/.slimebot/memory/.
func NewMemoryService(baseDir string) (*MemoryService, error) {
	store, err := NewFileMemoryStore(baseDir)
	if err != nil {
		return nil, fmt.Errorf("create file memory store: %w", err)
	}
	return &MemoryService{
		store:                       store,
		autoConsolidationMinEntries: 2,
	}, nil
}

// Shutdown closes the service.
func (m *MemoryService) Shutdown(ctx context.Context) error {
	if m == nil || m.store == nil {
		return nil
	}
	return m.store.Close()
}

// BuildMemoryContext builds memory context to inject into the chat prompt.
func (m *MemoryService) BuildMemoryContext(ctx context.Context, sessionID string, history []domain.Message) string {
	if m == nil || m.store == nil {
		return ""
	}
	return m.buildMemoryContext(ctx, sessionID, history)
}

// BuildSessionMemoryContextForPrompt is an alias for the legacy API.
func (m *MemoryService) BuildSessionMemoryContextForPrompt(ctx context.Context, sessionID string, history []domain.Message) string {
	return m.BuildMemoryContext(ctx, sessionID, history)
}

// BuildRecentHistory returns recent history messages (legacy API).
func (m *MemoryService) BuildRecentHistory(sessionID string, limit int) ([]domain.Message, error) {
	// Current design does not persist history here; return empty.
	return nil, nil
}

// QueryForAgent searches memories for Agent tool calls (cross-session).
func (m *MemoryService) QueryForAgent(ctx context.Context, sessionID string, query string, topK int) (MemoryQueryResult, error) {
	result := MemoryQueryResult{Query: strings.TrimSpace(query)}
	if result.Query == "" {
		return result, fmt.Errorf("memory_query query cannot be empty")
	}
	if topK <= 0 {
		topK = constants.MemoryToolDefaultTopK
	}

	entries, err := m.searchAllEntries(result.Query, topK)
	if err != nil {
		return result, fmt.Errorf("search memory: %w", err)
	}

	for _, entry := range entries {
		result.Hits = append(result.Hits, MemorySearchHit{
			Kind:      string(entry.Type),
			ID:        entry.Slug(),
			Title:     entry.Name,
			Summary:   truncateContent(entry.Content, 200),
			Score:     1.0, // bleve already ranks results
			CreatedAt: entry.Created,
		})
	}

	result.Output = buildMemoryQueryOutput(result.Query, nil, result.Hits)
	return result, nil
}

// EnqueueTurnMemory processes the model's memory payload.
// Parses JSON, deduplicates, then writes file-backed memories.
func (m *MemoryService) EnqueueTurnMemory(sessionID, assistantMessageID, rawMemoryPayload string) {
	if m == nil || m.store == nil {
		return
	}
	payload := strings.TrimSpace(rawMemoryPayload)
	if payload == "" {
		return
	}
	logging.Info("memory_process_start", "session", sessionID, "payload_len", len(payload))

	// Try to parse as MemoryEntry JSON.
	entry, err := parseMemoryPayload(payload)
	if err != nil {
		logging.Warn("memory_payload_parse_failed", "error", err)
		return
	}

	entry.SessionID = scopeForMemoryType(entry.Type, sessionID)

	if dup, dupErr := m.findConflictingMemory(entry); dupErr != nil {
		logging.Warn("memory_conflict_search_failed", "name", entry.Name, "error", dupErr)
	} else if dup != nil {
		entry.SetSlug(dup.Slug())
		entry.Created = dup.Created
		if entry.SessionID == "" {
			entry.SessionID = dup.SessionID
		}
	}

	if err := m.store.Save(entry); err != nil {
		logging.Warn("memory_save_failed", "name", entry.Name, "error", err)
		return
	}
}

// ReadEntrypoint reads MEMORY.md content.
func (m *MemoryService) ReadEntrypoint() string {
	if m == nil || m.store == nil {
		return ""
	}
	return m.store.ReadEntrypoint()
}

// Store returns the underlying FileMemoryStore (tests or advanced use).
func (m *MemoryService) Store() *FileMemoryStore {
	return m.store
}

// Consolidate runs one consolidation pass: merge fragments and remove redundancy.
func (m *MemoryService) Consolidate() (merged int, deleted int, err error) {
	if m == nil || m.store == nil {
		return 0, 0, nil
	}
	return NewConsolidator(m.store).Run()
}

// buildMemoryContext builds context from MEMORY.md index plus related memories.
// Uses conversation history as the search query and injects only selected memories,
// split into current-session context and long-term persistent context.
func (m *MemoryService) buildMemoryContext(ctx context.Context, sessionID string, history []domain.Message) string {
	if m == nil || m.store == nil {
		return ""
	}

	query := extractSearchQuery(history, 3)
	entries, err := m.selectContextEntries(sessionID, query, constants.MemoryContextTopK)
	if err != nil || len(entries) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<relevant_memories>\n")
	appendMemorySection(&b, "session_memory", "Current session context and active work.", entries)
	b.WriteString("</relevant_memories>")

	return b.String()
}

func (m *MemoryService) selectContextEntries(sessionID, query string, topK int) ([]*MemoryEntry, error) {
	if topK <= 0 {
		topK = constants.MemoryContextTopK
	}

	if strings.TrimSpace(query) == "" {
		return m.selectRecentEntries(sessionID, topK), nil
	}

	return m.searchRelevantEntries(sessionID, query, topK)
}

// extractSearchQuery builds search text from the last few turns.
// Concatenates user message text from the last lastN relevant turns.
func extractSearchQuery(history []domain.Message, lastN int) string {
	start := len(history) - lastN*2
	if start < 0 {
		start = 0
	}
	var parts []string
	for i := start; i < len(history); i++ {
		if history[i].Role == "user" {
			content := strings.TrimSpace(history[i].Content)
			if content != "" {
				parts = append(parts, content)
			}
		}
	}
	query := strings.Join(parts, " ")
	if len(query) > 200 {
		query = query[:200]
	}
	return query
}

func (m *MemoryService) searchRelevantEntries(sessionID, query string, topK int) ([]*MemoryEntry, error) {
	if sessionID == "" {
		return nil, nil
	}

	entries, err := m.store.SearchBySession(sessionID, query, topK*2)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return m.selectRecentEntries(sessionID, topK), nil
	}

	sortEntriesByRelevance(entries, query, sessionID)
	if len(entries) > topK {
		entries = entries[:topK]
	}
	return entries, nil
}

func (m *MemoryService) selectRecentEntries(sessionID string, topK int) []*MemoryEntry {
	if sessionID == "" {
		return nil
	}

	entries, err := m.store.Scan()
	if err != nil || len(entries) == 0 {
		return nil
	}

	var sessionEntries []*MemoryEntry
	for _, entry := range entries {
		if entry.SessionID == sessionID {
			sessionEntries = append(sessionEntries, entry)
		}
	}

	sortEntriesByRelevance(sessionEntries, "", sessionID)
	if len(sessionEntries) > topK {
		sessionEntries = sessionEntries[:topK]
	}
	return sessionEntries
}

func (m *MemoryService) findConflictingMemory(entry *MemoryEntry) (*MemoryEntry, error) {
	if m == nil || m.store == nil || entry == nil {
		return nil, nil
	}

	query := strings.TrimSpace(strings.Join([]string{entry.Name, entry.Description}, " "))
	if query == "" {
		query = entry.Name
	}
	candidates, err := m.store.Search(query, 8)
	if err != nil {
		return nil, err
	}

	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if candidate.Type != entry.Type {
			continue
		}
		if candidate.SessionID != entry.SessionID {
			continue
		}
		if candidate.Name == entry.Name {
			return candidate, nil
		}
		if !descriptiveEnoughForConflict(candidate.Description) || !descriptiveEnoughForConflict(entry.Description) {
			continue
		}
		if strings.TrimSpace(candidate.Content) == strings.TrimSpace(entry.Content) {
			continue
		}
		if shouldMerge(candidate, entry) {
			return candidate, nil
		}
	}
	return nil, nil
}

func splitEntriesByScope(entries []*MemoryEntry, sessionID string) (sessionScoped []*MemoryEntry, persistent []*MemoryEntry) {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if sessionID != "" && entry.SessionID == sessionID {
			sessionScoped = append(sessionScoped, entry)
			continue
		}
		persistent = append(persistent, entry)
	}
	return sessionScoped, persistent
}

func appendMemorySection(b *strings.Builder, tag, description string, entries []*MemoryEntry) {
	if len(entries) == 0 {
		return
	}
	b.WriteString("<")
	b.WriteString(tag)
	b.WriteString(">\n")
	if strings.TrimSpace(description) != "" {
		b.WriteString(description)
		b.WriteString("\n")
	}
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		header := fmt.Sprintf("## %s (%s)", entry.Name, entry.Type)
		if freshness := freshnessLabel(entry.Updated); freshness != "" {
			header += " " + freshness
		}
		b.WriteString(header)
		b.WriteString("\n")
		body := strings.TrimSpace(entry.Content)
		if body == "" {
			body = strings.TrimSpace(entry.Description)
		}
		if note := freshnessNotice(entry.Updated); note != "" {
			body = note + "\n" + body
		}
		b.WriteString(truncateContent(body, 500))
		b.WriteString("\n\n")
	}
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteString(">\n")
}

func filterPersistentEntries(entries []*MemoryEntry) []*MemoryEntry {
	var filtered []*MemoryEntry
	for _, entry := range entries {
		if entry == nil || entry.SessionID != "" {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func dedupeEntries(entries []*MemoryEntry) []*MemoryEntry {
	if len(entries) == 0 {
		return nil
	}
	result := make([]*MemoryEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if _, ok := seen[entry.Slug()]; ok {
			continue
		}
		seen[entry.Slug()] = struct{}{}
		result = append(result, entry)
	}
	return result
}

func sortEntriesByRelevance(entries []*MemoryEntry, query string, sessionID string) {
	query = strings.ToLower(strings.TrimSpace(query))
	sort.SliceStable(entries, func(i, j int) bool {
		left := memoryRelevanceScore(entries[i], query, sessionID)
		right := memoryRelevanceScore(entries[j], query, sessionID)
		if math.Abs(left-right) > 0.001 {
			return left > right
		}
		return entries[i].Updated.After(entries[j].Updated)
	})
}

func memoryRelevanceScore(entry *MemoryEntry, query string, sessionID string) float64 {
	if entry == nil {
		return -1
	}
	score := 0.0
	if sessionID != "" && entry.SessionID == sessionID {
		score += 10
	} else if entry.SessionID == "" {
		score += 5
	}

	if query != "" {
		tokens := tokenizeForMemoryMatch(query)
		name := strings.ToLower(entry.Name)
		description := strings.ToLower(entry.Description)
		content := strings.ToLower(entry.Content)
		for _, token := range tokens {
			switch {
			case strings.Contains(name, token):
				score += 4
			case strings.Contains(description, token):
				score += 2.5
			case strings.Contains(content, token):
				score += 1.25
			}
		}
	}

	ageHours := time.Since(entry.Updated).Hours()
	switch {
	case ageHours <= 24:
		score += 2
	case ageHours <= 24*7:
		score += 1
	}

	return score
}

func tokenizeForMemoryMatch(query string) []string {
	replacer := strings.NewReplacer(",", " ", ".", " ", "，", " ", "。", " ", "：", " ", ":", " ")
	fields := strings.Fields(replacer.Replace(strings.ToLower(strings.TrimSpace(query))))
	if len(fields) <= 8 {
		return fields
	}
	return fields[:8]
}

func descriptiveEnoughForConflict(description string) bool {
	description = strings.TrimSpace(description)
	if len([]rune(description)) >= 20 {
		return true
	}
	return len(strings.Fields(description)) >= 3
}

func scopeForMemoryType(t MemoryType, sessionID string) string {
	switch t {
	case MemoryTypeProject:
		return sessionID
	default:
		return ""
	}
}

func limitEntries(entries []*MemoryEntry, limit int) []*MemoryEntry {
	if len(entries) == 0 || limit <= 0 {
		return nil
	}
	if len(entries) <= limit {
		return entries
	}
	return entries[:limit]
}

// parseMemoryPayload parses model JSON payload into a MemoryEntry.
func parseMemoryPayload(raw string) (*MemoryEntry, error) {
	// Strip optional markdown code fence wrapper.
	cleaned := strings.TrimSpace(raw)
	if strings.HasPrefix(cleaned, "```") {
		// Strip opening ```json or ``` marker.
		firstNewline := strings.Index(cleaned, "\n")
		if firstNewline > 0 {
			cleaned = cleaned[firstNewline+1:]
		} else {
			cleaned = cleaned[3:]
		}
		// Strip closing ```.
		if idx := strings.LastIndex(cleaned, "```"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
		cleaned = strings.TrimSpace(cleaned)
	}

	// Parse as standard JSON.
	type payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Content     string `json:"content"`
	}

	var p payload
	if err := json.Unmarshal([]byte(cleaned), &p); err != nil {
		return nil, fmt.Errorf("unmarshal memory payload: %w", err)
	}

	if p.Name == "" {
		return nil, fmt.Errorf("empty name in memory payload")
	}

	memType, err := ParseMemoryType(p.Type)
	if err != nil {
		memType = MemoryTypeProject // default type
	}

	return &MemoryEntry{
		Name:        p.Name,
		Description: p.Description,
		Type:        memType,
		Content:     p.Content,
	}, nil
}

// freshnessLabel returns a freshness tag from update time (see Claude Code memoryAgeDays).
func freshnessLabel(updated time.Time) string {
	days := int(time.Since(updated).Hours() / 24)
	switch {
	case days <= 1:
		return ""
	case days <= 7:
		return fmt.Sprintf("[%d days ago]", days)
	case days <= 30:
		return fmt.Sprintf("[%d days ago, may be stale]", days)
	default:
		return fmt.Sprintf("[%d days ago, verify before use]", days)
	}
}

func freshnessNotice(updated time.Time) string {
	days := int(time.Since(updated).Hours() / 24)
	if days <= 1 {
		return ""
	}
	return fmt.Sprintf("This memory is %d days old. Verify code state or external facts before relying on it.", days)
}

// truncateContent truncates content to maxRunes runes.
func truncateContent(s string, maxRunes int) string {
	if len([]rune(s)) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// buildMemoryQueryOutput formats search hits as XML for the tool output.
func buildMemoryQueryOutput(query string, keywords []string, hits []MemorySearchHit) string {
	var b strings.Builder
	b.WriteString("<memory_query_result>\n")
	b.WriteString("query: ")
	b.WriteString(strings.TrimSpace(query))
	b.WriteString("\nkeywords: ")
	if len(keywords) == 0 {
		b.WriteString("(none)")
	} else {
		b.WriteString(strings.Join(keywords, ", "))
	}
	b.WriteString(fmt.Sprintf("\nhit_count: %d\n", len(hits)))
	if len(hits) == 0 {
		b.WriteString("No related memories found.\n</memory_query_result>")
		return b.String()
	}
	for idx, item := range hits {
		b.WriteString(fmt.Sprintf("- [%d] %s | %s | %.2f | %s\n", idx+1, item.Kind, item.Title, item.Score, item.CreatedAt.Format(time.RFC3339)))
		b.WriteString("  ")
		b.WriteString(strings.TrimSpace(item.Summary))
		b.WriteString("\n")
	}
	b.WriteString("</memory_query_result>")
	return strings.TrimSpace(b.String())
}

// searchAllEntries searches across all sessions without session filtering.
// Used by search_memory tool for cross-session retrieval.
func (m *MemoryService) searchAllEntries(query string, topK int) ([]*MemoryEntry, error) {
	entries, err := m.store.Search(query, topK*2)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	sortEntriesByRelevance(entries, query, "")
	if len(entries) > topK {
		entries = entries[:topK]
	}
	return entries, nil
}
