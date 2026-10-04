package contextsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	llm "slimebot/internal/services/llm"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

type summaryProvider struct {
	calls    int
	hook     func()
	bad      string
	finish   string
	err      error
	requests [][]llm.ChatMessage
	configs  []llm.ModelRuntimeConfig
}

var sourceNumber = regexp.MustCompile(`source=(\d+)`)

func (p *summaryProvider) StreamChatWithTools(ctx context.Context, c llm.ModelRuntimeConfig, msgs []llm.ChatMessage, tools []llm.ToolDef, cb llm.StreamCallbacks) (*llm.StreamResult, error) {
	p.calls++
	p.requests = append(p.requests, msgs)
	p.configs = append(p.configs, c)
	if len(tools) > 0 || c.Purpose != "compaction" || c.ThinkingLevel != "off" || c.MaxOutputTokens <= 0 {
		panic("unbounded summary request")
	}
	b, _ := RequestBudget(c)
	if Estimate(msgs, tools) > b.HardInput {
		panic("summary input overflow")
	}
	if p.hook != nil {
		p.hook()
	}
	if p.err != nil {
		return nil, p.err
	}
	match := sourceNumber.FindStringSubmatch(msgs[len(msgs)-1].Content)
	id, _ := strconv.ParseInt(match[1], 10, 64)
	summary := Summary{SchemaVersion: 1, Brief: "保留已确认决策、文件与待办", Goal: "完成当前任务", Artifacts: []string{"internal/example.go:42"}, SourceRefs: []int64{id}}
	out, _ := json.Marshal(summary)
	text := string(out)
	if p.bad != "" {
		text = p.bad
	}
	if cb.OnChunk != nil {
		if err := cb.OnChunk(text); err != nil {
			return nil, err
		}
	}
	finish := p.finish
	if finish == "" {
		finish = "stop"
	}
	return &llm.StreamResult{Type: llm.StreamResultText, FinishReason: finish, TokenUsage: &llm.TokenUsage{InputTokens: 123, OutputTokens: 45}}, nil
}
func config() llm.ModelRuntimeConfig {
	return llm.ModelRuntimeConfig{Provider: llm.ProviderOpenAI, Model: "test", ContextSize: 8192, MaxOutputTokens: 512}
}
func longHistory() []llm.ChatMessage {
	var ms []llm.ChatMessage
	for i := 0; i < 10; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		ms = append(ms, llm.ChatMessage{Role: role, Content: fmt.Sprintf("history %d ", i) + strings.Repeat("abc def ", 550)})
	}
	return append(ms, llm.ChatMessage{Role: "user", Content: "  最新请求：保留这个输入 🚀\n不要提交方案文档。  "})
}
func TestPrepareProtectsUserAndBoundsEverySummary(t *testing.T) {
	provider := &summaryProvider{}
	raw := longHistory()
	run := Ephemeral(llm.NewFactory(provider), raw)
	p, err := run.Prepare(context.Background(), config(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Compacted || provider.calls == 0 || provider.calls > 4 {
		t.Fatalf("expected bounded compaction: %+v calls=%d", p, provider.calls)
	}
	current := raw[len(raw)-1].Content
	found := false
	for _, m := range p.Messages {
		if m.Content == current {
			found = true
		}
		if strings.Contains(m.Content, "Historical context checkpoint") && m.Role != "user" {
			t.Fatal("checkpoint gained system authority")
		}
	}
	if !found || p.InputTokens > p.Budget.HardInput {
		t.Fatalf("current input lost or request overflow: %+v", p)
	}
	for _, request := range provider.requests {
		if strings.Contains(request[len(request)-1].Content, current) {
			t.Fatal("latest user was summarized")
		}
	}
	if len(run.snapshot.Entries) != len(raw) {
		t.Fatal("original transcript was deleted")
	}
}

func TestModelSwitchOnlyChangesHistoricalProjection(t *testing.T) {
	old := config()
	old.Model = "old-model"
	current := config()
	run := Ephemeral(nil, []llm.ChatMessage{{Role: "user", Content: "old request"}})
	run.routeHash = RouteHash(old)
	if err := run.Append(context.Background(), llm.ChatMessage{Role: "assistant", Content: "old answer", ReasoningContent: "private reasoning", ThinkingBlocks: []llm.ThinkingBlockInfo{{Thinking: "original", Signature: "signed-old"}}}); err != nil {
		t.Fatal(err)
	}
	raw := run.snapshot.Entries[1].Payload
	next := Ephemeral(nil, []llm.ChatMessage{{Role: "user", Content: "new request"}})
	next.snapshot.Entries[0].ID = 3
	next.snapshot.Entries = append(run.snapshot.Entries, next.snapshot.Entries...)
	p, err := next.Prepare(context.Background(), current, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Messages[1].Content != "old answer" || len(p.Messages[1].ThinkingBlocks) != 0 || p.Messages[1].ReasoningContent != "" || next.snapshot.Entries[1].Payload != raw {
		t.Fatal("model switch changed originals or replayed old signatures")
	}
	if _, err := run.Prepare(context.Background(), current, nil, false); err == nil {
		t.Fatal("mid-turn model switch accepted")
	}
	// A reloaded in-flight turn must obey the same constraint, even without run metadata.
	reloaded := Ephemeral(nil, nil)
	reloaded.snapshot.Entries = run.snapshot.Entries
	if _, err := reloaded.Prepare(context.Background(), current, nil, false); err == nil {
		t.Fatal("reloaded in-flight model switch accepted")
	}
}
func TestLongCurrentTurnRetainsClosedStepsAndSignatures(t *testing.T) {
	provider := &summaryProvider{}
	user := "当前轮任务不能消失"
	run := Ephemeral(llm.NewFactory(provider), []llm.ChatMessage{{Role: "user", Content: user}})
	compacted := 0
	for step := 0; step < 45; step++ {
		a, b := fmt.Sprintf("a-%d", step), fmt.Sprintf("b-%d", step)
		signature := fmt.Sprintf("signature-%d", step)
		messages := []llm.ChatMessage{{Role: "assistant", ReasoningContent: "exact reasoning", ThinkingBlocks: []llm.ThinkingBlockInfo{{Thinking: "immutable", Signature: signature}}, ToolCalls: []llm.ToolCallInfo{{ID: a, Name: "test"}, {ID: b, Name: "test"}}}, {Role: "tool", ToolCallID: b, Content: strings.Repeat("long log ", 500) + "\nERROR E104 src/test.go:42"}, {Role: "tool", ToolCallID: a, Content: strings.Repeat("other log ", 500)}}
		if err := run.Append(context.Background(), messages...); err != nil {
			t.Fatal(err)
		}
		p, err := run.Prepare(context.Background(), config(), nil, false)
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		if p.Compacted {
			compacted++
		}
		if err := validatePairing(p.Messages); err != nil {
			t.Fatal(err)
		}
		last := p.Messages[len(p.Messages)-3]
		if last.ToolCalls[0].ID != a || last.ThinkingBlocks[0].Signature != signature || last.ReasoningContent != "exact reasoning" {
			t.Fatal("latest step changed")
		}
		found := false
		for _, m := range p.Messages {
			found = found || m.Content == user
		}
		if !found {
			t.Fatal("current user was replaced")
		}
	}
	if compacted < 10 {
		t.Fatalf("insufficient repeated compaction coverage: %d", compacted)
	}
}
func TestQualityFailuresNeverReplaceRawHistory(t *testing.T) {
	for _, tc := range []struct{ name, bad, finish string }{{"invalid_json", "not json", ""}, {"empty", "{}", ""}, {"truncated", "", "length"}, {"refused", "", "content_filter"}, {"invalid_source", `{"schema_version":1,"brief":"bad","source_refs":[99999]}`, ""}, {"oversized", `{"schema_version":1,"brief":"` + strings.Repeat("巨大", 4000) + `","source_refs":[1]}`, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &summaryProvider{bad: tc.bad, finish: tc.finish}
			raw := longHistory()
			run := Ephemeral(llm.NewFactory(provider), raw)
			if _, err := run.Prepare(context.Background(), config(), nil, false); err == nil {
				t.Fatal("expected rejection")
			}
			if len(run.snapshot.Checkpoints) != 0 || len(run.snapshot.Entries) != len(raw) {
				t.Fatal("rejected summary changed history")
			}
		})
	}
}
func TestSoftFailureAndOversizedCurrentInput(t *testing.T) {
	provider := &summaryProvider{err: errors.New("temporary network failure")}
	messages := []llm.ChatMessage{{Role: "assistant", Content: strings.Repeat("abc ", 1500)}, {Role: "assistant", Content: strings.Repeat("abc ", 1500)}, {Role: "assistant", Content: strings.Repeat("abc ", 1500)}, {Role: "user", Content: strings.Repeat("current ", 50)}}
	run := Ephemeral(llm.NewFactory(provider), messages)
	p, err := run.Prepare(context.Background(), config(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Compacted || provider.calls != 1 {
		t.Fatalf("soft failure not preserved: %+v calls=%d", p, provider.calls)
	}
	huge := Ephemeral(llm.NewFactory(provider), []llm.ChatMessage{{Role: "user", Content: strings.Repeat("不可以截断", 2000)}})
	if _, err := huge.Prepare(context.Background(), config(), nil, false); err == nil {
		t.Fatal("oversized protected input should fail")
	}
	if provider.calls != 1 {
		t.Fatal("protected input sent to summarizer")
	}
}
func TestToolSchemaAndOutputReserveAreCounted(t *testing.T) {
	r := Ephemeral(nil, []llm.ChatMessage{{Role: "user", Content: "hello"}})
	c := config()
	c.MaxOutputTokens = 6000
	tools := []llm.ToolDef{{Name: "big", Description: strings.Repeat("schema ", 1000), Parameters: map[string]any{"type": "object"}}}
	if _, err := r.Prepare(context.Background(), c, tools, false); err == nil {
		t.Fatal("schema/output budget ignored")
	}
}
func TestPrunerKeepsMiddleDiagnosticsUnicodeAndOriginal(t *testing.T) {
	raw := "```text\n" + strings.Repeat("普通日志 🚀 hello\n", 300) + "ERROR E101 src/a.go:41\nERROR E102 src/a.go:42\n" + strings.Repeat("普通日志\n", 300) + "exit code: 1\n```"
	out := PruneText(raw, 800, "17")
	for _, want := range []string{"E101", "E102", "src/a.go:41", "src/a.go:42", "exit code: 1", "source=17"} {
		if !strings.Contains(out, want) {
			t.Fatalf("lost %s: %s", want, out)
		}
	}
	if !utf8.ValidString(out) || strings.Count(out, "```")%2 != 0 || EstimateText(out) > 800 || len(out) >= len(raw) {
		t.Fatal("invalid pruned text")
	}
	mixed := llm.ChatMessage{Role: "tool", ContentParts: []llm.ChatMessageContentPart{{Type: "text", Text: raw}, {Type: "image", ImageURL: "test"}}}
	pruned := pruneMessage(mixed, 800, "17")
	if pruned.ContentParts[0].Text != raw || pruned.ContentParts[1].ImageURL != "test" {
		t.Fatal("rich block changed")
	}
}
func TestScopedReadIsBoundedAndUsesOriginal(t *testing.T) {
	raw := strings.Repeat("raw original 🚀\n", 500)
	r := Ephemeral(nil, []llm.ChatMessage{{Role: "tool", Content: raw}})
	text, err := r.Read(1, 20, 128)
	if err != nil || !strings.Contains(text, "next_offset=") || !strings.Contains(text, "raw original") {
		t.Fatalf("read: %s %v", text, err)
	}
	if EstimateText(text) > 200 {
		t.Fatal("read not bounded")
	}
	if _, err := r.Read(999, 0, 128); err == nil {
		t.Fatal("foreign source accepted")
	}
	if _, err := r.Read(1, -1, 128); err == nil {
		t.Fatal("negative offset accepted")
	}
}
func TestCancelledSummaryHasNoCheckpoint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &summaryProvider{hook: cancel}
	r := Ephemeral(llm.NewFactory(provider), longHistory())
	if _, err := r.Prepare(ctx, config(), nil, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
	if len(r.snapshot.Checkpoints) > 0 {
		t.Fatal("cancel committed summary")
	}
}

func persistentRun(t *testing.T, provider *summaryProvider) (*repositories.Repository, *Run, string, string) {
	t.Helper()
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "ctx_v2"))
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "context")
	if err != nil {
		t.Fatal(err)
	}
	var seeds []domain.ContextSeed
	last := ""
	for _, m := range longHistory() {
		record, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{SessionID: session.ID, Role: m.Role, Content: m.Content})
		if err != nil {
			t.Fatal(err)
		}
		last = record.ID
		seeds = append(seeds, domain.ContextSeed{MessageID: record.ID, Seq: record.Seq, SourceHash: domain.ContextMessageHash(*record), Fidelity: "legacy_replay", Messages: []llm.ChatMessage{m}})
	}
	seeds[len(seeds)-1].Fidelity = "exact"
	service := New(repo, llm.NewFactory(provider))
	run, err := service.Begin(ctx, session.ID, "request", nil, seeds)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(run.Close)
	return repo, run, session.ID, last
}
func TestCheckpointCASAndEditInvalidation(t *testing.T) {
	provider := &summaryProvider{}
	repo, run, scope, userID := persistentRun(t, provider)
	provider.hook = func() {
		provider.hook = nil
		if _, err := repo.UpdateUserMessageAndPruneAfter(context.Background(), scope, userID, "edited input"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := run.Prepare(context.Background(), config(), nil, false); !errors.Is(err, repositories.ErrContextConflict) {
		t.Fatalf("expected edit conflict: %v", err)
	}
	snapshot, err := repo.GetContextSnapshot(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Checkpoints) != 0 || len(snapshot.Entries) != len(longHistory())-1 {
		t.Fatal("edit retained stale projection")
	}
	if err := run.Append(context.Background(), llm.ChatMessage{Role: "assistant", Content: "stale"}); !errors.Is(err, repositories.ErrContextConflict) {
		t.Fatal("stale append accepted")
	}
}
func TestCommittedCheckpointReloadAndReadOnlySnapshot(t *testing.T) {
	provider := &summaryProvider{}
	repo, run, scope, _ := persistentRun(t, provider)
	p, err := run.Prepare(context.Background(), config(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Compacted {
		t.Fatal("not compacted")
	}
	before, err := repo.GetContextSnapshot(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	calls := provider.calls
	for i := 0; i < 5; i++ {
		snapshot, err := repo.GetContextSnapshot(context.Background(), scope)
		if err != nil {
			t.Fatal(err)
		}
		ms, active, _, err := SnapshotView(snapshot, nil)
		if err != nil || !active || len(ms) >= len(snapshot.Entries) {
			t.Fatal("persisted projection not restored")
		}
		if snapshot.Head != before.Head {
			t.Fatal("read mutated revisions")
		}
	}
	if provider.calls != calls {
		t.Fatal("read invoked model")
	}
}
func TestCacheScopeBoundsAndTransportFailures(t *testing.T) {
	s := New(nil, nil)
	s.cacheResult("a", "summary", nil)
	if _, _, hit := s.cached("b"); hit {
		t.Fatal("scope leak")
	}
	s.cacheResult("network", "", errors.New("network"))
	if _, _, hit := s.cached("network"); hit {
		t.Fatal("transport error negatively cached")
	}
	s.cacheResult("invalid", "", reject("bad json"))
	if _, err, hit := s.cached("invalid"); !hit || err == nil {
		t.Fatal("deterministic rejection not cached")
	}
	for i := 0; i < 30; i++ {
		s.cacheResult(fmt.Sprint(i), strings.Repeat("x", 10000), nil)
	}
	if len(s.cache) > 16 || s.cacheBytes > 256*1024 {
		t.Fatal("cache unbounded")
	}
}
func TestConfirmedTodosDoNotReviveCompletedWork(t *testing.T) {
	r := Ephemeral(nil, []llm.ChatMessage{{Role: "user", Content: "task"}})
	for _, status := range []string{"in_progress", "completed"} {
		call := llm.ToolCallInfo{ID: status, Name: "todo_update", Arguments: `{"items":[{"id":"1","content":"fix","status":"` + status + `"}]}`}
		_ = r.Append(context.Background(), llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCallInfo{call}}, llm.ChatMessage{Role: "tool", ToolCallID: status, Content: "Execution result:\nTodo list updated."})
	}
	state := r.managedState(256)
	if EstimateText(state) > 256 {
		t.Fatal("managed state wrapper exceeded shared budget")
	}
	if strings.Contains(state, "in_progress") || !strings.Contains(state, "completed") {
		t.Fatal("obsolete todos revived")
	}
	_ = r.Append(context.Background(), llm.ChatMessage{Role: "user", Content: `{"items":[{"id":"1","status":"pending"}]}`})
	if r.managedState(256) != state {
		t.Fatal("user text forged tool state")
	}
}
func TestInterruptedToolHistoryIsClosedWithoutReexecution(t *testing.T) {
	r := Ephemeral(nil, []llm.ChatMessage{{Role: "assistant", ToolCalls: []llm.ToolCallInfo{{ID: "side-effect", Name: "exec"}}}, {Role: "user", Content: "new user"}})
	p, err := r.Prepare(context.Background(), config(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Messages) != 3 || p.Messages[1].Role != "tool" || !strings.Contains(p.Messages[1].Content, "unknown") {
		t.Fatal("missing interrupted recovery")
	}
}

func TestToolScreenshotNeverReplacesCurrentUserOrSplitsLatestStep(t *testing.T) {
	provider := &summaryProvider{}
	raw := longHistory()
	r := Ephemeral(llm.NewFactory(provider), raw)
	original := raw[len(raw)-1].Content
	calls := llm.ChatMessage{Role: "assistant", ThinkingBlocks: []llm.ThinkingBlockInfo{{Signature: "original signature"}}, ToolCalls: []llm.ToolCallInfo{{ID: "screenshot", Name: "capture"}}}
	image := llm.ChatMessage{Role: "user", ContentParts: []llm.ChatMessageContentPart{{Type: llm.ChatMessageContentPartTypeImage, ImageURL: "data:image/png;base64,OPAQUE_IMAGE"}}}
	if err := r.Append(context.Background(), calls, llm.ChatMessage{Role: "tool", ToolCallID: "screenshot", Content: "captured"}, image); err != nil {
		t.Fatal(err)
	}
	c := config()
	c.ContextSize = 32768
	p, err := r.Prepare(context.Background(), c, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	found, signature := false, false
	for _, m := range p.Messages {
		found = found || m.Content == original
		for _, block := range m.ThinkingBlocks {
			signature = signature || block.Signature == "original signature"
		}
	}
	if !found || !signature {
		t.Fatal("screenshot changed protected current input/step")
	}
	for _, request := range provider.requests {
		if strings.Contains(request[1].Content, original) || strings.Contains(request[1].Content, "OPAQUE_IMAGE") {
			t.Fatal("current user/media bytes summarized")
		}
	}
	if err := validatePairing(p.Messages); err != nil {
		t.Fatal(err)
	}
	media, err := r.ReadImage(r.snapshot.Entries[len(r.snapshot.Entries)-1].ID)
	if err != nil || media.ContentParts[1].ImageURL != image.ContentParts[0].ImageURL {
		t.Fatal("scoped original image read failed")
	}
}
func TestVisionBudgetDoesNotCountURLCharacters(t *testing.T) {
	m := llm.ChatMessage{Role: "user", ContentParts: []llm.ChatMessageContentPart{{Type: llm.ChatMessageContentPartTypeImage, ImageURL: "x"}}}
	if Estimate([]llm.ChatMessage{m}, nil) < 8192 {
		t.Fatal("vision omitted from budget")
	}
	r := Ephemeral(nil, []llm.ChatMessage{m})
	if _, err := r.Prepare(context.Background(), config(), nil, false); err == nil {
		t.Fatal("protected image overflow was sent")
	}
}
func TestOpaqueHistoricalMediaNeverBecomesTextTranscript(t *testing.T) {
	provider := &summaryProvider{}
	raw := []llm.ChatMessage{{Role: "user", ContentParts: []llm.ChatMessageContentPart{{Type: llm.ChatMessageContentPartTypeImage, ImageURL: strings.Repeat("PRIVATE_BASE64", 10000)}}}, {Role: "assistant", Content: "Image previously inspected; found ERROR E200."}, {Role: "user", Content: "continue original task"}}
	r := Ephemeral(llm.NewFactory(provider), raw)
	c := config()
	c.ContextSize = 32768
	if _, err := r.Prepare(context.Background(), c, nil, true); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatal("historical image not compacted")
	}
	if strings.Contains(provider.requests[0][1].Content, "PRIVATE_BASE64") {
		t.Fatal("opaque binary expanded into summary text")
	}
	if !strings.Contains(provider.requests[0][1].Content, "visual/audio contents are not described") {
		t.Fatal("opaque image provenance omitted")
	}
}
func TestCompactionOffRetainsRawDataAndGuards(t *testing.T) {
	t.Setenv("SLIMEBOT_CONTEXT_COMPACTION", "off")
	provider := &summaryProvider{}
	r := Ephemeral(llm.NewFactory(provider), longHistory())
	if _, err := r.Prepare(context.Background(), config(), nil, false); err == nil {
		t.Fatal("disabled compaction bypassed hard guard")
	}
	if provider.calls != 0 {
		t.Fatal("disabled compaction called summary model")
	}
}
