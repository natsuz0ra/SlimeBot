package chat

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"slimebot/internal/domain"
	"slimebot/internal/logging"

	llmsvc "slimebot/internal/services/llm"
)

const (
	titleMaxUserRunes   = 500
	titleMaxOutputRunes = 20
	titleTimeout        = 30 * time.Second
)

var initialSessionNames = map[string]struct{}{
	"":            {},
	"New Chat":    {},
	"New Session": {},
	"新会话":         {},
	"未命名会话":       {},
}

var titleSystemPrompt = `You are a concise title generator. Given the user's opening message, generate a short title that summarizes the conversation topic.

Return ONLY a JSON object: {"title":"..."}

Rules:
- Keep the title under 20 characters (or similarly concise in non-Latin scripts)
- Match the user's language
- Prefer "action + object" format (e.g., "优化登录流程", "Fix mobile layout")
- No quotes inside the title, no line breaks, no extra tags
- Be specific, not vague

Good: {"title": "优化登录页面布局"}
Good: {"title": "Fix mobile layout bug"}
Good: {"title": "实现文件上传功能"}
Bad (too vague): {"title": "聊天"}
Bad (too long): {"title": "调查并修复移动端登录按钮无法响应点击的问题"}`

// sessionTitleUpdater abstracts session title persistence.
type sessionTitleUpdater interface {
	UpdateSessionTitle(ctx context.Context, id, name string) (bool, error)
}

// titleGenerator makes standalone LLM calls to generate session titles.
type titleGenerator struct {
	factory *llmsvc.Factory
	store   sessionTitleUpdater

	mu        sync.Mutex
	attempted map[string]titleAttemptStatus // session IDs with in-flight or completed title generation
}

type titleAttemptStatus int

const (
	titleAttemptInFlight titleAttemptStatus = iota + 1
	titleAttemptCompleted
)

// newTitleGenerator creates a title generator with the given dependencies.
func newTitleGenerator(factory *llmsvc.Factory, store sessionTitleUpdater) *titleGenerator {
	return &titleGenerator{
		factory:   factory,
		store:     store,
		attempted: make(map[string]titleAttemptStatus),
	}
}

// generate makes one LLM call to produce a session title.
func (g *titleGenerator) generate(ctx context.Context, modelConfig llmsvc.ModelRuntimeConfig, userMsg string) (string, error) {
	messages := []llmsvc.ChatMessage{
		{Role: "system", Content: titleSystemPrompt},
		{Role: "user", Content: buildTitleUserPrompt(userMsg)},
	}

	provider := g.factory.GetProvider(modelConfig.Provider)

	// Collect full response via accumulator pattern (same as agent loop).
	var buf strings.Builder
	cfg := modelConfig
	cfg.ThinkingLevel = "" // no thinking for title generation

	_, err := provider.StreamChatWithTools(ctx, cfg, messages, nil, llmsvc.StreamCallbacks{
		OnChunk: func(chunk string) error {
			buf.WriteString(chunk)
			return nil
		},
	})
	if err != nil {
		return "", err
	}

	return parseTitleFromJSON(buf.String()), nil
}

// markAttempted records that title generation completed for this session.
func (g *titleGenerator) markAttempted(sessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.attempted[sessionID] = titleAttemptCompleted
	if len(g.attempted) > 4096 {
		// Evict half to prevent unbounded growth.
		i := 0
		for id := range g.attempted {
			delete(g.attempted, id)
			i++
			if i >= 2048 {
				break
			}
		}
	}
}

// hasBeenAttempted returns true if title generation is already running or completed for this session.
func (g *titleGenerator) hasBeenAttempted(sessionID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.attempted[sessionID]
	return ok
}

// beginAttempt marks title generation as in-flight. It returns false if another
// attempt is already running or a title has already been completed.
func (g *titleGenerator) beginAttempt(sessionID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.attempted[sessionID]; ok {
		return false
	}
	g.attempted[sessionID] = titleAttemptInFlight
	return true
}

// finishAttempt records the final attempt state. Failed attempts are released so
// a later turn can retry; successful attempts remain completed.
func (g *titleGenerator) finishAttempt(sessionID string, completed bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if completed {
		g.attempted[sessionID] = titleAttemptCompleted
		return
	}
	if g.attempted[sessionID] == titleAttemptInFlight {
		delete(g.attempted, sessionID)
	}
}

// maybeGenerateTitleAsync launches a background goroutine to generate a title
// when the session is still in its initial state and the title is not locked.
func (s *ChatService) maybeGenerateTitleAsync(
	session *domain.Session,
	modelConfig llmsvc.ModelRuntimeConfig,
	userContent string,
	onTitleGenerated func(sessionID, title string),
) {
	if session == nil {
		logging.Info("title_generation_skipped", "reason", "nil_session")
		return
	}
	if session.IsTitleLocked {
		logging.Info("title_generation_skipped", "session", session.ID, "reason", "title_locked")
		return
	}
	if s.titleGen == nil {
		logging.Info("title_generation_skipped", "session", session.ID, "reason", "generator_unavailable")
		return
	}
	if s.titleGen.hasBeenAttempted(session.ID) {
		logging.Info("title_generation_skipped", "session", session.ID, "reason", "already_attempted")
		return
	}
	if !isInitialSessionName(session.Name) {
		logging.Info("title_generation_skipped", "session", session.ID, "reason", "non_initial_name", "name", session.Name)
		return
	}

	userMsg := truncateForTitleContext(userContent, titleMaxUserRunes)
	if strings.TrimSpace(userMsg) == "" {
		logging.Info("title_generation_skipped", "session", session.ID, "reason", "empty_user_message")
		return
	}

	gen := s.titleGen
	sid := session.ID
	if !gen.beginAttempt(sid) {
		logging.Info("title_generation_skipped", "session", sid, "reason", "already_attempted")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), titleTimeout)
	defer cancel()

	title, err := gen.generate(ctx, modelConfig, userMsg)
	if err != nil {
		gen.finishAttempt(sid, false)
		logging.Info("title_generation_failed", "session", sid, "error", err.Error())
		return
	}
	if title == "" {
		gen.finishAttempt(sid, false)
		logging.Info("title_generation_skipped", "session", sid, "reason", "empty_generated_title")
		return
	}

	updated, err := gen.store.UpdateSessionTitle(ctx, sid, title)
	if err != nil {
		gen.finishAttempt(sid, false)
		logging.Info("title_persist_failed", "session", sid, "error", err.Error())
		return
	}
	gen.finishAttempt(sid, updated)
	logging.Info("title_generation_persisted", "session", sid, "updated", updated, "title", title)
	if updated && onTitleGenerated != nil {
		onTitleGenerated(sid, title)
	}
}

// isInitialSessionName returns true if the name looks like a default/untitled session.
// Matches all known i18n defaults used by the frontend and backend.
func isInitialSessionName(name string) bool {
	normalized := strings.TrimSpace(name)
	_, ok := initialSessionNames[normalized]
	return ok
}

// buildTitleUserPrompt constructs the user prompt for title generation.
func buildTitleUserPrompt(userMsg string) string {
	var b strings.Builder
	b.WriteString("User: ")
	b.WriteString(userMsg)
	return b.String()
}

// parseTitleFromJSON extracts the "title" field from a JSON response.
// Returns empty string if parsing fails.
func parseTitleFromJSON(raw string) string {
	text := strings.TrimSpace(raw)

	// Try to find JSON object in the response.
	start := strings.Index(text, "{")
	if start < 0 {
		return cleanGeneratedTitle(text)
	}
	end := strings.LastIndex(text, "}")
	if end <= start {
		return cleanGeneratedTitle(text)
	}

	jsonStr := text[start : end+1]
	var result struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return cleanGeneratedTitle(text)
	}

	return cleanGeneratedTitle(result.Title)
}

// cleanGeneratedTitle sanitizes a raw title: strip whitespace/quotes, truncate.
func cleanGeneratedTitle(raw string) string {
	title := strings.TrimSpace(raw)
	title = strings.Trim(title, "\"'“”")
	title = strings.ReplaceAll(title, "\r", "")
	title = strings.ReplaceAll(title, "\n", "")
	title = truncateRunes(title, titleMaxOutputRunes)
	return strings.TrimSpace(title)
}

// truncateForTitleContext truncates input to maxRunes from the end (recent context wins).
func truncateForTitleContext(input string, maxRunes int) string {
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return input
	}
	return string(runes[len(runes)-maxRunes:])
}

func truncateRunes(input string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return input
	}
	return string(runes[:maxRunes])
}
