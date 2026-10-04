package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"regexp"
	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	contextsvc "slimebot/internal/services/context"
	llm "slimebot/internal/services/llm"
	"strconv"
	"strings"
	"testing"
	"time"
)

type overflowProvider struct {
	mainCalls, summaryCalls int
	failure                 error
	alwaysFail              bool
}

func (p *overflowProvider) StreamChatWithTools(ctx context.Context, c llm.ModelRuntimeConfig, ms []llm.ChatMessage, defs []llm.ToolDef, cb llm.StreamCallbacks) (*llm.StreamResult, error) {
	if c.Purpose == "compaction" {
		p.summaryCalls++
		source := regexp.MustCompile(`source=(\d+)`).FindStringSubmatch(ms[len(ms)-1].Content)
		id, _ := strconv.ParseInt(source[1], 10, 64)
		b, _ := json.Marshal(contextsvc.Summary{SchemaVersion: 1, Brief: "保留历史任务和已确认结果", SourceRefs: []int64{id}})
		if err := cb.OnChunk(string(b)); err != nil {
			return nil, err
		}
		return &llm.StreamResult{FinishReason: "stop"}, nil
	}
	p.mainCalls++
	if p.mainCalls == 1 {
		return &llm.StreamResult{Type: llm.StreamResultToolCalls, ToolCalls: []llm.ToolCallInfo{{ID: "one-side-effect", Name: "todo_update", Arguments: `{"items":[{"id":"1","content":"run once","status":"completed"}]}`}}}, nil
	}
	if p.mainCalls == 2 || p.alwaysFail {
		return nil, p.failure
	}
	if cb.OnChunk != nil {
		_ = cb.OnChunk("done")
	}
	return &llm.StreamResult{FinishReason: "stop"}, nil
}
func TestOverflowRetriesOnlyModelRequestAndNeverReexecutesTools(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		failure                error
		always                 bool
		wantCalls, wantSummary int
		wantErr                bool
	}{{"recover", llm.ClassifyContextError(errors.New("context_length_exceeded")), false, 3, 1, false}, {"bounded", llm.ClassifyContextError(errors.New("context_length_exceeded")), true, 3, 1, true}, {"not_overflow", errors.New("HTTP 400 invalid tool schema"), false, 2, 0, true}} {
		t.Run(tc.name, func(t *testing.T) {
			p := &overflowProvider{failure: tc.failure, alwaysFail: tc.always}
			agent := NewAgentService(llm.NewFactory(p), nil, nil)
			ms := []llm.ChatMessage{{Role: "assistant", Content: strings.Repeat("old history ", 500)}, {Role: "user", Content: "run once"}}
			executed := 0
			_, err := agent.RunAgentLoop(context.Background(), llm.ModelRuntimeConfig{ContextSize: 32768}, "scope", ms, nil, map[string]struct{}{}, AgentCallbacks{OnTodoUpdate: func(TodoUpdate) error { executed++; return nil }}, AgentLoopOptions{})
			if (err != nil) != tc.wantErr || p.mainCalls != tc.wantCalls || p.summaryCalls != tc.wantSummary || executed != 1 {
				t.Fatalf("err=%v calls=%d summaries=%d executions=%d", err, p.mainCalls, p.summaryCalls, executed)
			}
		})
	}
}
func TestContextUsageReadsDoNotCallModelsOrWriteDatabase(t *testing.T) {
	db := repositories.NewSQLiteDBTest(t, "context_readonly")
	repo := repositories.New(db)
	ctx := context.Background()
	session, _ := repo.CreateSession(ctx, "readonly")
	user, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{SessionID: session.ID, Role: "user", Content: strings.Repeat("当前输入", 3000)})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSessionContextSummary(ctx, &domain.SessionContextSummary{SessionID: session.ID, Summary: "legacy swallowed current input", SummarizedUntilSeq: user.Seq}); err != nil {
		t.Fatal(err)
	}
	writes := 0
	for _, register := range []func(string, func(*gorm.DB)) error{db.Callback().Create().Before("gorm:create").Register, db.Callback().Update().Before("gorm:update").Register, db.Callback().Delete().Before("gorm:delete").Register} {
		if err := register("readonly_probe", func(*gorm.DB) { writes++ }); err != nil {
			t.Fatal(err)
		}
	}
	p := &overflowProvider{}
	service := NewChatService(repo, nil, llm.NewFactory(p), nil, nil)
	for i := 0; i < 4; i++ {
		usage, err := service.BuildContextUsage(ctx, session.ID, llm.ModelRuntimeConfig{ContextSize: 8192})
		if err != nil {
			t.Fatal(err)
		}
		if usage.Source != "estimated" || usage.IsCompacted {
			t.Fatalf("wrong readonly projection: %+v", usage)
		}
	}
	if writes != 0 || p.mainCalls != 0 || p.summaryCalls != 0 {
		t.Fatalf("GET had side effects: writes=%d calls=%d", writes, p.mainCalls)
	}
	ms, err := service.BuildContextMessages(ctx, session.ID, llm.ModelRuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if ms[len(ms)-1].Content != user.Content {
		t.Fatal("legacy summary still swallowed latest user")
	}
}
func TestLargeHistoryIncludesLatestRegardlessOfTimestamp(t *testing.T) {
	db := repositories.NewSQLiteDBTest(t, "context_long_history")
	repo := repositories.New(db)
	ctx := context.Background()
	session, _ := repo.CreateSession(ctx, "long")
	records := make([]domain.Message, 10005)
	for i := range records {
		records[i] = domain.Message{ID: uuid.NewString(), SessionID: session.ID, Seq: int64(i + 1), Role: "user", Content: fmt.Sprint("message-", i), CreatedAt: time.Now().Add(-time.Duration(i) * time.Second)}
	}
	if err := db.CreateInBatches(records, 500).Error; err != nil {
		t.Fatal(err)
	}
	service := NewChatService(repo, nil, nil, nil, nil)
	ms, err := service.BuildContextMessages(ctx, session.ID, llm.ModelRuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if ms[len(ms)-1].Content != records[len(records)-1].Content {
		t.Fatal("10000-row cutoff or timestamp ordering lost current input")
	}
}
func TestPersistentRawStepsReplaceFlattenedUIReplay(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, _ := repo.CreateSession(ctx, "exact")
	model, _ := repo.CreateLLMConfig(ctx, domain.LLMConfig{Name: "fake", Provider: llm.ProviderOpenAI, BaseURL: "http://fake", APIKey: "key", Model: "fake", ContextSize: 100000})
	provider := &reasoningToolIterationProvider{}
	service := NewChatService(repo, nil, llm.NewFactory(provider), nil, nil)
	_, err := service.HandleChatStream(ctx, session.ID, "actual-turn", "do it", "", model.ID, nil, "off", false, "", "", AgentCallbacks{OnChunk: func(string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := repo.GetContextSnapshot(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	reasoning := false
	for _, e := range snapshot.Entries {
		m, err := e.Message()
		if err != nil {
			t.Fatal(err)
		}
		if len(m.ToolCalls) > 0 && m.ReasoningContent != "" {
			reasoning = true
		}
		if e.Fidelity != "exact" {
			t.Fatal("new model steps imported as legacy")
		}
	}
	if !reasoning {
		t.Fatal("reasoning missing from durable transcript")
	}
	msgs, err := service.BuildContextMessages(ctx, session.ID, llm.ModelRuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	replayed := false
	for _, m := range msgs {
		replayed = replayed || m.ReasoningContent != ""
	}
	if !replayed {
		t.Fatal("exact steps replaced by UI aggregation")
	}
}
