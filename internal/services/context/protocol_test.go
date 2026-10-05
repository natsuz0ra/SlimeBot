package contextsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	anthropic "slimebot/internal/services/anthropic"
	llm "slimebot/internal/services/llm"
	openai "slimebot/internal/services/openai"
)

func protocolFactory() *llm.Factory {
	factory := llm.NewFactory(openai.NewOpenAIClient())
	factory.Register(llm.ProviderAnthropic, anthropic.NewAnthropicClient())
	return factory
}

const contextAnswer = "1.34.0|CTX-134|not-committed|current-ok"

func protocolHistory() []llm.ChatMessage {
	return []llm.ChatMessage{
		{Role: "user", Content: "Release version is 1.34.0. Verification code is CTX-134. The design document must remain local, not committed. Output file is desktop/proof.txt."},
		{Role: "assistant", ToolCalls: []llm.ToolCallInfo{{ID: "old_read", Name: "read_file", Arguments: `{"path":"desktop/proof.txt"}`}}},
		{Role: "tool", ToolCallID: "old_read", Content: "desktop/proof.txt:1 verification complete"},
		{Role: "assistant", Content: strings.Repeat("An old verification step completed successfully. No pending changes in this step. ", 120)},
		{Role: "user", Content: "Using the historical facts and latest tool result, reply exactly VERSION|CODE|not-committed|current-ok. Replace only VERSION and CODE with the historical release version and verification code. The third and fourth fields are literal strings. Do not call tools."},
		{Role: "assistant", ReasoningContent: "Previously verified the file.", ToolCalls: []llm.ToolCallInfo{{ID: "current_read", Name: "read_file", Arguments: `{"path":"desktop/proof.txt"}`}}},
		{Role: "tool", ToolCallID: "current_read", Content: "current-ok"},
	}
}

func protocolTools() []llm.ToolDef {
	return []llm.ToolDef{{Name: "read_file", Description: "Read a file without modifying it.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}, "additionalProperties": false}}}
}

func newProtocolRun(t *testing.T, factory *llm.Factory, history []llm.ChatMessage) (*gorm.DB, *repositories.Repository, *Service, *Run) {
	t.Helper()
	db := repositories.NewSQLiteDBTest(t, "context_protocol")
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	repo := repositories.New(db)
	session, err := repo.CreateSession(context.Background(), "compaction protocol")
	if err != nil {
		t.Fatal(err)
	}
	seeds := make([]domain.ContextSeed, 0, len(history))
	for _, m := range history {
		record, err := repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{SessionID: session.ID, Role: m.Role, Content: m.Content})
		if err != nil {
			t.Fatal(err)
		}
		seeds = append(seeds, domain.ContextSeed{MessageID: record.ID, Seq: record.Seq, SourceHash: domain.ContextMessageHash(*record), Fidelity: "exact", Messages: []llm.ChatMessage{m}})
	}
	service := New(repo, factory)
	run, err := service.Begin(context.Background(), session.ID, "prepare", []llm.ChatMessage{{Role: "system", Content: "Use the verified historical facts. Follow the latest user request precisely."}}, seeds)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(run.Close)
	return db, repo, service, run
}

func protocolReloadSeeds(t *testing.T, repo *repositories.Repository, scope string) []domain.ContextSeed {
	t.Helper()
	history, err := repo.ListAllSessionMessages(context.Background(), scope, -1)
	if err != nil {
		t.Fatal(err)
	}
	var seeds []domain.ContextSeed
	for _, record := range history {
		seeds = append(seeds, domain.ContextSeed{MessageID: record.ID, Seq: record.Seq, SourceHash: domain.ContextMessageHash(record), Fidelity: "legacy_replay", Messages: []llm.ChatMessage{{Role: record.Role, Content: record.Content}}})
	}
	return seeds
}

// Use the production adapters and persistence, not a stub Provider. All failure
// cases are local; the optional live test below makes two requests per protocol.
func TestCompactionProtocolLifecycle(t *testing.T) {
	for _, protocol := range []string{llm.ProviderOpenAI, llm.ProviderDeepSeek, llm.ProviderAnthropic} {
		for _, mode := range []string{"success", "pressure", "malformed", "invalid_source", "truncated", "length", "cancelled"} {
			t.Run(protocol+"/"+mode, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					n := requests.Add(1)
					data, _ := json.Marshal(body)
					if n == 1 {
						if tools, ok := body["tools"].([]any); ok && len(tools) > 0 {
							t.Error("compaction exposed executable tools")
						}
						if protocol != llm.ProviderOpenAI {
							thinking, _ := body["thinking"].(map[string]any)
							if thinking["type"] != "disabled" {
								t.Error("compaction thinking not disabled")
							}
						}
						if !strings.Contains(string(data), "Summarize historical facts") || strings.Contains(string(data), "current-ok") || strings.Contains(string(data), "sig-current") || strings.Contains(string(data), "private reasoning") {
							t.Error("summary input boundary or private thinking leaked")
						}
						match := sourceNumber.FindStringSubmatch(string(data))
						if len(match) != 2 {
							t.Error("summary sources missing")
							w.WriteHeader(400)
							return
						}
						id, _ := strconv.ParseInt(match[1], 10, 64)
						if mode == "invalid_source" {
							id = 9999999
						}
						summary, _ := json.Marshal(Summary{SchemaVersion: 1, Brief: "Version 1.34.0, code CTX-134; the design document is not committed.", Goal: "Verify release", Constraints: []string{"Keep design document local, not committed"}, Artifacts: []string{"desktop/proof.txt"}, SourceRefs: []int64{id}})
						text := string(summary)
						if mode == "malformed" {
							text = "not JSON"
						}
						if mode == "cancelled" {
							cancel()
							return
						}
						writeContextResponse(w, protocol, text, mode)
					} else {
						if !strings.Contains(string(data), "Historical context checkpoint") || !strings.Contains(string(data), "CTX-134") || !strings.Contains(string(data), "current_read") || !strings.Contains(string(data), "current-ok") {
							t.Error("checkpoint/current tool pair missing on wire")
						}
						if protocol == llm.ProviderAnthropic && !strings.Contains(string(data), `"signature":"sig-current"`) {
							t.Error("current-turn signature lost")
						}
						if protocol != llm.ProviderAnthropic && !strings.Contains(string(data), `"reasoning_content":"private reasoning"`) {
							t.Error("current-turn reasoning lost")
						}
						writeContextResponse(w, protocol, contextAnswer, "success")
					}
				}))
				defer server.Close()
				factory := protocolFactory()
				history := protocolHistory()
				history[5].ReasoningContent = "private reasoning"
				history[5].ThinkingBlocks = []llm.ThinkingBlockInfo{{Thinking: "private reasoning", Signature: "sig-current"}}
				if mode == "pressure" {
					history[3].Content = strings.Repeat("An old verification step completed successfully. No pending changes in this step. ", 80)
					history = append(append(append([]llm.ChatMessage{}, history[:4]...), history[3], history[3]), history[4:]...)
				}

				db, repo, _, run := newProtocolRun(t, factory, history)
				before, err := repo.GetContextSnapshot(context.Background(), run.scope)
				if err != nil {
					t.Fatal(err)
				}
				c := llm.ModelRuntimeConfig{Provider: protocol, BaseURL: server.URL, APIKey: "test", Model: "test", ContextSize: 8192, MaxOutputTokens: 256, ThinkingLevel: "off"}
				var settled atomic.Int32
				ctx = llm.WithRequestObserver(ctx, func(context.Context, llm.ModelRuntimeConfig, []llm.ChatMessage, []llm.ToolDef) (func(*llm.StreamResult, error) error, error) {
					return func(_ *llm.StreamResult, e error) error { settled.Add(1); return e }, nil
				})
				prepared, err := run.Prepare(ctx, c, protocolTools(), mode != "pressure")
				after, snapshotErr := repo.GetContextSnapshot(context.Background(), run.scope)
				if snapshotErr != nil {
					t.Fatal(snapshotErr)
				}
				if !reflect.DeepEqual(before.Entries, after.Entries) {
					t.Fatal("raw history changed during compaction")
				}
				var attempts []domain.ContextCheckpoint
				if err := db.Where("scope_id = ?", run.scope).Find(&attempts).Error; err != nil {
					t.Fatal(err)
				}
				if len(attempts) != 1 || requests.Load() != 1 || settled.Load() != 1 {
					t.Fatalf("unbounded/missing summary: attempts=%d requests=%d settled=%d", len(attempts), requests.Load(), settled.Load())
				}
				if mode != "success" && mode != "pressure" {
					status := "failed"
					if mode == "cancelled" {
						status = "cancelled"
					}
					if err == nil || len(after.Checkpoints) != 0 || attempts[0].Status != status || attempts[0].Active {
						t.Fatalf("failure installed checkpoint: err=%v attempt=%+v", err, attempts[0])
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if !prepared.Compacted || len(after.Checkpoints) != 1 || prepared.InputTokens >= prepared.BeforeTokens || attempts[0].UsageJSON == "" {
					t.Fatalf("checkpoint/usage missing: %+v", prepared)
				}
				if err := validatePairing(prepared.Messages); err != nil {
					t.Fatal(err)
				}
				// Simulate a new process with an empty summary cache: the durable checkpoint
				// must reload without another model call, and preserve the current turn.
				reloadedService := New(repo, factory)
				reloaded, err := reloadedService.Begin(ctx, run.scope, "resume", run.prefix, protocolReloadSeeds(t, repo, run.scope))
				if err != nil {
					t.Fatal(err)
				}
				defer reloaded.Close()
				p, err := reloaded.Prepare(ctx, c, protocolTools(), false)
				if err != nil {
					t.Fatal(err)
				}
				if p.Compacted || requests.Load() != 1 || !reflect.DeepEqual(p.Messages, prepared.Messages) {
					t.Fatal("reload regenerated summary or changed projection")
				}
				result, err := factory.GetProvider(protocol).StreamChatWithTools(ctx, c, p.Messages, protocolTools(), llm.StreamCallbacks{})
				if err != nil {
					t.Fatal(err)
				}
				if result.AssistantMessage.Content != contextAnswer || requests.Load() != 2 || settled.Load() != 2 {
					t.Fatalf("resume failed: %+v", result)
				}
			})
		}
	}
}

func writeContextResponse(w http.ResponseWriter, protocol, text, mode string) {
	w.Header().Set("Content-Type", "text/event-stream")
	if protocol == llm.ProviderAnthropic {
		event := func(name string, data map[string]any) {
			data["type"] = name
			b, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
		}
		event("message_start", map[string]any{"message": map[string]any{"id": "ctx", "type": "message", "role": "assistant", "usage": map[string]any{"input_tokens": 100, "cache_read_input_tokens": 20, "output_tokens": 0}}})
		event("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		for _, part := range []string{text[:len(text)/2], text[len(text)/2:]} {
			event("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "text_delta", "text": part}})
		}
		event("content_block_stop", map[string]any{"index": 0})
		if mode == "truncated" {
			return
		}
		finish := "end_turn"
		if mode == "length" {
			finish = "max_tokens"
		}
		event("message_delta", map[string]any{"delta": map[string]any{"stop_reason": finish}, "usage": map[string]any{"output_tokens": 80}})
		event("message_stop", map[string]any{})
	} else {
		for i, part := range []string{text[:len(text)/2], text[len(text)/2:]} {
			var finish any
			if i == 1 && mode != "truncated" {
				finish = "stop"
				if mode == "length" {
					finish = "length"
				}
			}
			b, _ := json.Marshal(map[string]any{"id": "ctx", "object": "chat.completion.chunk", "created": 1, "model": "test", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": part}, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		if mode == "truncated" {
			return
		}
		fmt.Fprint(w, "data: {\"id\":\"ctx\",\"object\":\"chat.completion.chunk\",\"model\":\"test\",\"choices\":[],\"usage\":{\"prompt_tokens\":120,\"completion_tokens\":80,\"total_tokens\":200}}\n\ndata: [DONE]\n\n")
	}
}

// Opt in only with a loopback credential-injecting proxy. CI never contacts a
// real provider; credentials are not read or stored by this test.
func TestCompactionRealProtocols(t *testing.T) {
	endpoint := os.Getenv("SLIMEBOT_REAL_CONTEXT_ENDPOINT")
	if endpoint == "" {
		t.Skip("requires explicitly configured live-test proxy")
	}
	if !strings.HasPrefix(endpoint, "http://127.0.0.1:") {
		t.Fatal("live test must use a bounded loopback proxy")
	}
	for _, protocol := range []string{llm.ProviderOpenAI, llm.ProviderDeepSeek, llm.ProviderAnthropic} {
		t.Run(protocol, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			factory := protocolFactory()
			_, repo, _, run := newProtocolRun(t, factory, protocolHistory())
			c := llm.ModelRuntimeConfig{Provider: protocol, BaseURL: endpoint, APIKey: "live-test-placeholder", Model: "deepseek-flash", ContextSize: 16384, MaxOutputTokens: 512, ThinkingLevel: "off"}
			var calls atomic.Int32
			ctx = llm.WithRequestObserver(ctx, func(context.Context, llm.ModelRuntimeConfig, []llm.ChatMessage, []llm.ToolDef) (func(*llm.StreamResult, error) error, error) {
				calls.Add(1)
				return func(_ *llm.StreamResult, err error) error { return err }, nil
			})
			p, err := run.Prepare(ctx, c, protocolTools(), true)
			if err != nil {
				t.Fatal(err)
			}
			if !p.Compacted || calls.Load() != 1 {
				t.Fatalf("expected exactly one compaction: calls=%d", calls.Load())
			}
			reloadedService := New(repo, factory)
			reloaded, err := reloadedService.Begin(ctx, run.scope, "resume", run.prefix, protocolReloadSeeds(t, repo, run.scope))
			if err != nil {
				t.Fatal(err)
			}
			defer reloaded.Close()
			p, err = reloaded.Prepare(ctx, c, protocolTools(), false)
			if err != nil {
				t.Fatal(err)
			}
			result, err := factory.GetProvider(protocol).StreamChatWithTools(ctx, c, p.Messages, protocolTools(), llm.StreamCallbacks{})
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(result.AssistantMessage.Content) != contextAnswer || calls.Load() != 2 {
				t.Fatalf("facts lost after compression: response=%q calls=%d", result.AssistantMessage.Content, calls.Load())
			}
			snapshot, err := repo.GetContextSnapshot(ctx, run.scope)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Checkpoints) != 1 || len(snapshot.Entries) != len(protocolHistory()) {
				t.Fatal("checkpoint/raw history persistence failed")
			}
			var summaryUsage llm.TokenUsage
			if err := json.Unmarshal([]byte(snapshot.Checkpoints[0].UsageJSON), &summaryUsage); err != nil {
				t.Fatal(err)
			}
			if summaryUsage.OutputTokens > 1024 || result.TokenUsage == nil || result.TokenUsage.OutputTokens > c.MaxOutputTokens {
				t.Fatal("provider ignored output reserve")
			}

			t.Logf("protocol=%s calls=%d checkpoint=%d→%d tokens resume_usage=%+v", protocol, calls.Load(), snapshot.Checkpoints[0].BeforeTokens, snapshot.Checkpoints[0].AfterTokens, result.TokenUsage)
		})
	}
}

func TestStatusFailureFinishesCompactionAttempt(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			provider := &summaryProvider{}
			db, repo, _, run := newProtocolRun(t, llm.NewFactory(provider), protocolHistory())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("status transport disconnected")
			run.OnStatus = func(string) error {
				if cancelled {
					cancel()
				}
				return failure
			}
			if _, err := run.Prepare(ctx, config(), nil, true); !errors.Is(err, failure) {
				t.Fatalf("status error lost: %v", err)
			}
			var attempts []domain.ContextCheckpoint
			if err := db.Where("scope_id = ?", run.scope).Find(&attempts).Error; err != nil {
				t.Fatal(err)
			}
			status := "failed"
			if cancelled {
				status = "cancelled"
			}
			snapshot, err := repo.GetContextSnapshot(context.Background(), run.scope)
			if err != nil {
				t.Fatal(err)
			}
			if len(attempts) != 1 || attempts[0].Status != status || provider.calls != 0 || len(snapshot.Checkpoints) != 0 {
				t.Fatalf("unfinished attempt after status failure: %+v calls=%d", attempts, provider.calls)
			}
		})
	}
}

func TestCompactionRetainsHumanAnchorBeforeAgentRelay(t *testing.T) {
	for _, kind := range []string{"direct_user", "delegated_task", "human_input"} {
		t.Run(kind, func(t *testing.T) {
			provider := &summaryProvider{}
			history := protocolHistory()
			history[4].SourceKind = kind
			history = append(history, llm.ChatMessage{Role: "user", SourceKind: "agent_result", Content: "Child agent completed its investigation."})
			_, repo, _, run := newProtocolRun(t, llm.NewFactory(provider), history)
			p, err := run.Prepare(context.Background(), config(), nil, true)
			if err != nil {
				t.Fatal(err)
			}
			if !p.Compacted {
				t.Fatal("compaction did not run")
			}
			found := false
			for _, m := range p.Messages {
				if m.Content == history[4].Content {
					found = true
				}
			}
			if !found {
				t.Fatal("agent relay caused current user request to be compressed")
			}
			snapshot, err := repo.GetContextSnapshot(context.Background(), run.scope)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := project(snapshot.Entries, snapshot.Checkpoints); err != nil {
				t.Fatalf("invalid durable projection: %v", err)
			}
		})
	}
}

func TestMissingSummaryProviderFailsWithoutPanic(t *testing.T) {
	db, _, _, run := newProtocolRun(t, llm.NewFactory(nil), protocolHistory())
	if _, err := run.Prepare(context.Background(), config(), nil, true); err == nil {
		t.Fatal("missing provider accepted")
	}
	var attempts []domain.ContextCheckpoint
	if err := db.Where("scope_id = ?", run.scope).Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Status != "failed" {
		t.Fatalf("unfinished missing-provider attempt: %+v", attempts)
	}
}
