package memory

import (
	"context"
	"fmt"
	"slimebot/internal/domain"
	"strings"
	"testing"
	"time"
)

func TestMemoryService_EnqueueTurnMemory_ConflictOverride(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewMemoryService(dir)
	if err != nil {
		t.Fatalf("NewMemoryService: %v", err)
	}
	defer svc.Shutdown(context.Background())

	first := `{"name":"Go Build","description":"CI uses make test in repo","type":"project","content":"Pipeline currently uses make test."}`
	second := `{"name":"Build Pipeline","description":"CI uses make test in repository","type":"project","content":"Final decision: CI uses go test ./..."}`

	svc.EnqueueTurnMemory("session-1", "", first)
	svc.EnqueueTurnMemory("session-1", "", second)

	entries, err := svc.Store().Scan()
	if err != nil {
		t.Fatalf("scan entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one merged/overridden memory, got %d", len(entries))
	}
	if entries[0].Content != "Final decision: CI uses go test ./..." {
		t.Fatalf("expected latest content to override, got: %q", entries[0].Content)
	}
}

func TestMemoryService_QueryForAgent_ReRankByRecency(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewMemoryService(dir)
	if err != nil {
		t.Fatalf("NewMemoryService: %v", err)
	}
	defer svc.Shutdown(context.Background())

	oldPayload := `{"name":"Old Plan","description":"deployment uses canary","type":"project","content":"deployment uses canary and health checks"}`
	newPayload := `{"name":"Latest Plan","description":"deployment uses canary","type":"project","content":"deployment uses canary and health checks"}`

	svc.EnqueueTurnMemory("session-1", "", oldPayload)
	time.Sleep(20 * time.Millisecond)
	svc.EnqueueTurnMemory("session-1", "", newPayload)

	result, err := svc.QueryForAgent(context.Background(), "session-1", "deployment canary health checks", 5)
	if err != nil {
		t.Fatalf("QueryForAgent: %v", err)
	}
	if len(result.Hits) < 2 {
		t.Fatalf("expected at least two hits, got %d", len(result.Hits))
	}
	if result.Hits[0].Title != "Latest Plan" {
		t.Fatalf("expected newest memory ranked first, got %q", result.Hits[0].Title)
	}
}

func TestMemoryService_TryAutoConsolidate_Gates(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewMemoryService(dir)
	if err != nil {
		t.Fatalf("NewMemoryService: %v", err)
	}
	defer svc.Shutdown(context.Background())

	svc.ConfigureAutoConsolidation(true, 30*time.Minute, 2)

	svc.EnqueueTurnMemory("session-1", "", `{"name":"A","description":"same topic","type":"project","content":"c1"}`)
	ran, _, _, err := svc.TryAutoConsolidate("test-min-entries")
	if err != nil {
		t.Fatalf("TryAutoConsolidate(min-entries): %v", err)
	}
	if ran {
		t.Fatalf("expected consolidation skipped when entries < minEntries")
	}

	svc.EnqueueTurnMemory("session-1", "", `{"name":"B","description":"same topic","type":"project","content":"c2"}`)
	ran, _, _, err = svc.TryAutoConsolidate("test-first-run")
	if err != nil {
		t.Fatalf("TryAutoConsolidate(first-run): %v", err)
	}
	if !ran {
		t.Fatalf("expected first eligible consolidation to run")
	}

	ran, _, _, err = svc.TryAutoConsolidate("test-interval-gate")
	if err != nil {
		t.Fatalf("TryAutoConsolidate(interval-gate): %v", err)
	}
	if ran {
		t.Fatalf("expected consolidation skipped by min interval")
	}
}

func TestMemoryService_TryAutoConsolidate_ConcurrentGuard(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewMemoryService(dir)
	if err != nil {
		t.Fatalf("NewMemoryService: %v", err)
	}
	defer svc.Shutdown(context.Background())

	// Create enough entries to pass minEntries gate.
	for i := 0; i < 3; i++ {
		payload := fmt.Sprintf(`{"name":"Item-%d","description":"topic-%d","type":"project","content":"c-%d"}`, i, i, i)
		svc.EnqueueTurnMemory("session-1", "", payload)
	}
	svc.ConfigureAutoConsolidation(true, 0, 1)
	svc.SetConsolidateHookForTest(func() {
		time.Sleep(80 * time.Millisecond)
	})

	done := make(chan bool, 2)
	go func() {
		ran, _, _, _ := svc.TryAutoConsolidate("g1")
		done <- ran
	}()
	go func() {
		ran, _, _, _ := svc.TryAutoConsolidate("g2")
		done <- ran
	}()

	r1 := <-done
	r2 := <-done
	if (r1 && r2) || (!r1 && !r2) {
		t.Fatalf("expected exactly one run due to concurrency guard, got r1=%v r2=%v", r1, r2)
	}
}

func TestMemoryService_EnqueueTurnMemory_ScopesPersistentTypesGlobally(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewMemoryService(dir)
	if err != nil {
		t.Fatalf("NewMemoryService: %v", err)
	}
	defer svc.Shutdown(context.Background())

	svc.EnqueueTurnMemory("session-1", "", `{"name":"Reply Style","description":"user prefers concise Chinese replies","type":"user","content":"Always reply in concise Chinese unless asked otherwise."}`)
	svc.EnqueueTurnMemory("session-1", "", `{"name":"Deploy Plan","description":"current deployment plan","type":"project","content":"Use canary release for this session."}`)

	entries, err := svc.Store().Scan()
	if err != nil {
		t.Fatalf("scan entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	byName := map[string]*MemoryEntry{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}

	if got := byName["Reply Style"].SessionID; got != "" {
		t.Fatalf("user memory should be global, got session_id=%q", got)
	}
	if got := byName["Deploy Plan"].SessionID; got != "session-1" {
		t.Fatalf("project memory should stay session-scoped, got session_id=%q", got)
	}
}

func TestMemoryService_BuildMemoryContext_SelectsSessionAndGlobalMemoriesWithoutLeakingOtherSessions(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewMemoryService(dir)
	if err != nil {
		t.Fatalf("NewMemoryService: %v", err)
	}
	defer svc.Shutdown(context.Background())

	if err := svc.Store().Save(&MemoryEntry{
		Name:        "Global Reply Preference",
		Description: "user prefers concise Chinese replies",
		Type:        MemoryTypeUser,
		Content:     "Answer in concise Chinese by default.",
	}); err != nil {
		t.Fatalf("save global memory: %v", err)
	}
	if err := svc.Store().Save(&MemoryEntry{
		Name:        "Current Session Deploy Plan",
		Description: "deploy with canary and health checks",
		Type:        MemoryTypeProject,
		SessionID:   "session-1",
		Content:     "Use canary release and verify health checks before full rollout.",
	}); err != nil {
		t.Fatalf("save session memory: %v", err)
	}
	if err := svc.Store().Save(&MemoryEntry{
		Name:        "Other Session Secret Plan",
		Description: "deploy with blue green",
		Type:        MemoryTypeProject,
		SessionID:   "session-2",
		Content:     "This should never leak into session-1 context.",
	}); err != nil {
		t.Fatalf("save other session memory: %v", err)
	}

	history := []domain.Message{
		{Role: "user", Content: "请用中文简洁说明 canary 发布和健康检查步骤"},
	}
	contextText := svc.BuildMemoryContext(context.Background(), "session-1", history)

	if strings.Contains(contextText, "Global Reply Preference") {
		t.Fatalf("global memory should NOT appear in context, use search_memory tool instead, got: %s", contextText)
	}
	if !strings.Contains(contextText, "Current Session Deploy Plan") {
		t.Fatalf("expected current-session memory to be included, got: %s", contextText)
	}
	if strings.Contains(contextText, "Other Session Secret Plan") {
		t.Fatalf("other-session memory leaked into context: %s", contextText)
	}
	if strings.Contains(contextText, "<memory_index>") {
		t.Fatalf("memory context should not inject the full MEMORY.md manifest anymore: %s", contextText)
	}
}
