package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"slimebot/internal/services/llm"
)

type testStore struct {
	mu   sync.Mutex
	raw  string
	fail bool
}

func (s *testStore) GetSetting(context.Context, string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return "", errors.New("unavailable")
	}
	return s.raw, nil
}
func (s *testStore) SetSetting(_ context.Context, _, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw = v
	return nil
}

type fakeProvider struct {
	run func(context.Context, llm.ModelRuntimeConfig, []llm.ChatMessage, []llm.ToolDef, llm.StreamCallbacks) (*llm.StreamResult, error)
}

func (p fakeProvider) StreamChatWithTools(c context.Context, m llm.ModelRuntimeConfig, v []llm.ChatMessage, d []llm.ToolDef, b llm.StreamCallbacks) (*llm.StreamResult, error) {
	return p.run(c, m, v, d, b)
}
func setup(t *testing.T) (*Service, *testStore, context.Context) {
	t.Helper()
	store := &testStore{}
	s := NewService(store, t.TempDir())
	t.Cleanup(s.Close)
	c := DefaultConfig()
	c.Enabled = true
	if err := s.Update(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return s, store, llm.WithPrivacyScope(context.Background(), "session-one", "request-one", "test")
}

func TestProviderMasksWholeRequestAndRestoresJSON(t *testing.T) {
	s, _, ctx := setup(t)
	const secret = "MySecret123"
	messages := []llm.ChatMessage{{Role: "system", Content: "An earlier unlabelled value: " + secret}, {Role: "user", Content: "数据库密码是 " + secret + "，请生成连接命令。"}, {Role: "tool", ToolCallID: "call_13800138000", Content: `{"password":"MySecret123","timestamp":13800138000}`}}
	original, _ := json.Marshal(messages)
	defs := []llm.ToolDef{{Name: "file_write", Description: "Use " + secret, Parameters: map[string]any{
		"type":       "object",
		"properties": map[string]any{"password": map[string]any{"type": "string", "description": "A value: " + secret, "enum": []string{"unchanged"}}},
	}}}
	originalDefs, _ := json.Marshal(defs)
	var displayed strings.Builder
	next := fakeProvider{run: func(_ context.Context, cfg llm.ModelRuntimeConfig, sent []llm.ChatMessage, tools []llm.ToolDef, cb llm.StreamCallbacks) (*llm.StreamResult, error) {
		b, _ := json.Marshal(sent)
		if strings.Contains(string(b), secret) {
			t.Fatalf("secret sent: %s", b)
		}
		if cfg.APIKey != "authentication-is-intact" || sent[2].ToolCallID != messages[2].ToolCallID {
			t.Fatal("protocol identifiers changed")
		}
		toolData, _ := json.Marshal(tools)
		if strings.Contains(string(toolData), secret) || !strings.Contains(string(toolData), "unchanged") {
			t.Fatal("tool descriptions or schema changed incorrectly")
		}
		p := placeholderRE.FindString(sent[1].Content)
		if p == "" {
			t.Fatal("no placeholder")
		}
		output := "命令：" + p + "。"
		for _, byteValue := range []byte(output) {
			if err := cb.OnChunk(string([]byte{byteValue})); err != nil {
				return nil, err
			}
		}
		args, _ := json.Marshal(map[string]any{"value": "quote \" " + p + "\n\\end", "n": 7, "enabled": true})
		call := llm.ToolCallInfo{ID: "call_13800138000", Name: "file_write", Arguments: string(args)}
		return &llm.StreamResult{Type: llm.StreamResultToolCalls, ToolCalls: []llm.ToolCallInfo{call}, AssistantMessage: llm.ChatMessage{Role: "assistant", Content: output, ToolCalls: []llm.ToolCallInfo{call}, ThinkingBlocks: []llm.ThinkingBlockInfo{{Thinking: p, Signature: "signed-wire-bytes"}}}}, nil
	}}
	result, err := s.Wrap(next).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{APIKey: "authentication-is-intact"}, messages, defs, llm.StreamCallbacks{OnChunk: func(v string) error { displayed.WriteString(v); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if displayed.String() != "命令："+secret+"。" || result.AssistantMessage.Content != displayed.String() {
		t.Fatalf("stream/result mismatch: %q", displayed.String())
	}
	var args map[string]any
	if json.Unmarshal([]byte(result.ToolCalls[0].Arguments), &args) != nil || args["value"] != "quote \" "+secret+"\n\\end" || args["n"] != float64(7) || args["enabled"] != true {
		t.Fatalf("invalid restored arguments: %#v", args)
	}
	if !strings.Contains(result.AssistantMessage.ThinkingBlocks[0].Thinking, "{{SB_") || result.AssistantMessage.ThinkingBlocks[0].Signature != "signed-wire-bytes" {
		t.Fatal("signed thinking changed")
	}
	after, _ := json.Marshal(messages)
	afterDefs, _ := json.Marshal(defs)
	if string(after) != string(original) || string(afterDefs) != string(originalDefs) {
		t.Fatal("original inputs mutated")
	}
}

func TestFailClosedAndWarn(t *testing.T) {
	for _, mode := range []string{"block", "warn", "settings_error", "model_error", "panic"} {
		t.Run(mode, func(t *testing.T) {
			s, store, ctx := setup(t)
			c, _ := s.GetConfig(ctx)
			if mode == "block" {
				c.Actions["PASSWORD"] = Block
			}
			if mode == "warn" {
				c.Actions["PASSWORD"] = Warn
			}
			if mode == "model_error" {
				c.ModelEnabled = true
				s.detect = func(context.Context, string, float64) ([]span, error) { return nil, errors.New("bad model") }
			}
			if mode == "panic" {
				c.ModelEnabled = true
				s.detect = func(context.Context, string, float64) ([]span, error) { panic("sensitive input must not enter errors") }
			}
			_ = s.Update(ctx, c)
			if mode == "settings_error" {
				store.fail = true
			}
			called := false
			next := fakeProvider{run: func(_ context.Context, _ llm.ModelRuntimeConfig, m []llm.ChatMessage, _ []llm.ToolDef, _ llm.StreamCallbacks) (*llm.StreamResult, error) {
				called = true
				if mode == "warn" && !strings.Contains(m[0].Content, "MySecret123") {
					t.Fatal("warn must send original")
				}
				return &llm.StreamResult{}, nil
			}}
			_, err := s.Wrap(next).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{}, []llm.ChatMessage{{Content: "密码是 MySecret123"}}, nil, llm.StreamCallbacks{})
			if mode == "warn" {
				if err != nil || !called {
					t.Fatalf("warn: %v", err)
				}
			} else {
				if err == nil || called {
					t.Fatalf("failed open: %v, called=%v", err, called)
				}
				if strings.Contains(err.Error(), "sensitive input") {
					t.Fatal("panic content exposed")
				}
			}
		})
	}
}

func TestUnknownPlaceholderCannotReachToolExecution(t *testing.T) {
	s, _, ctx := setup(t)
	next := fakeProvider{run: func(context.Context, llm.ModelRuntimeConfig, []llm.ChatMessage, []llm.ToolDef, llm.StreamCallbacks) (*llm.StreamResult, error) {
		return &llm.StreamResult{Type: llm.StreamResultToolCalls, ToolCalls: []llm.ToolCallInfo{{Arguments: `{"value":"{{SB_PASSWORD_0123456789abcdef0123456789abcdef}}"}`}}}, nil
	}}
	if result, err := s.Wrap(next).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{}, []llm.ChatMessage{{Content: "hello"}}, nil, llm.StreamCallbacks{}); err == nil || result != nil {
		t.Fatal("unrestored tool arguments returned")
	}
}

func TestScopeIsolationAndRestart(t *testing.T) {
	s, store, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	first, _ := s.begin(ctx, c, false)
	p, err := first.maskText(ctx, "DB_PASSWORD=IsolatedSecret123", "")
	if err != nil {
		t.Fatal(err)
	}
	first.finish(false)
	otherCtx := llm.WithPrivacyScope(ctx, "another-session", "", "test")
	other, _ := s.begin(otherCtx, c, false)
	if _, err := other.restore(p); err == nil {
		t.Fatal("cross-session restore allowed")
	}
	restarted := NewService(store, s.dir)
	defer restarted.Close()
	again, _ := restarted.begin(ctx, c, false)
	if _, err := again.restore(p); err == nil {
		t.Fatal("plaintext map survived restart")
	}
	rebuilt, _ := again.maskText(ctx, "DB_PASSWORD=IsolatedSecret123", "")
	if rebuilt != p {
		t.Fatal("unstable placeholder after restart")
	}
	if restored, err := again.restore(p); err != nil || restored != "DB_PASSWORD=IsolatedSecret123" {
		t.Fatal("rebuild failed")
	}
}

func TestStreamingEverySplit(t *testing.T) {
	s, _, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	tx, _ := s.begin(ctx, c, true)
	p, _ := tx.placeholder("SECRET", "包含中文和引号\"换行\n")
	input := "前缀" + p + "后缀"
	for split := 0; split <= len(input); split++ {
		var out strings.Builder
		stream := restoreStream{tx: tx, emit: func(v string) error { out.WriteString(v); return nil }}
		if err := stream.Write(input[:split]); err != nil {
			t.Fatal(err)
		}
		if err := stream.Write(input[split:]); err != nil {
			t.Fatal(err)
		}
		if err := stream.Finish(); err != nil {
			t.Fatal(err)
		}
		if out.String() != "前缀包含中文和引号\"换行\n后缀" {
			t.Fatalf("split %d: %q", split, out.String())
		}
	}
}

func TestPreviewDoesNotLearnValues(t *testing.T) {
	s, _, ctx := setup(t)
	result, err := s.Preview(ctx, "DB_PASSWORD=PreviewSecret123")
	if err != nil || len(result.Hits) == 0 {
		t.Fatal(err)
	}
	snapshot, _ := s.Snapshot(ctx)
	if len(snapshot.Records) != 0 || len(s.scopes) != 0 {
		t.Fatal("preview changed live state")
	}
}

func TestKnownValuesAndJSONTypes(t *testing.T) {
	s, _, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	tx, _ := s.begin(ctx, c, true)
	for _, input := range []string{`{"phone":13800138000,"text":"13800138000","ok":true}`, "DB_PASSWORD=secret123abc", "now secret123abc is elsewhere"} {
		out, err := tx.maskText(ctx, input, "")
		if err != nil {
			t.Fatal(err)
		}
		if out == input {
			t.Fatalf("not masked: %s", input)
		}
		if strings.HasPrefix(input, "{") {
			v, _ := decodeJSON(out)
			if !reflect.DeepEqual(v.(map[string]any)["phone"], json.Number("13800138000")) {
				t.Fatal("numeric type changed")
			}
		}
	}
}

func TestOptionalModelTimeoutAndCache(t *testing.T) {
	s, _, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	c.ModelEnabled = true
	_ = s.Update(ctx, c)
	calls := 0
	s.detect = func(context.Context, string, float64) ([]span, error) {
		calls++
		return []span{{0, len("张三"), "PERSON", "model"}}, nil
	}
	next := fakeProvider{run: func(_ context.Context, _ llm.ModelRuntimeConfig, m []llm.ChatMessage, _ []llm.ToolDef, _ llm.StreamCallbacks) (*llm.StreamResult, error) {
		if strings.Contains(m[0].Content, "张三") {
			t.Fatal("model find leaked")
		}
		return &llm.StreamResult{}, nil
	}}
	if _, err := s.Wrap(next).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{}, []llm.ChatMessage{{Content: "张三是联系人"}}, nil, llm.StreamCallbacks{}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("same text scanned %d times", calls)
	}
	s.detect = func(ctx context.Context, _ string, _ float64) ([]span, error) { <-ctx.Done(); return nil, ctx.Err() }
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if _, err := s.Wrap(next).StreamChatWithTools(short, llm.ModelRuntimeConfig{}, []llm.ChatMessage{{Content: "another request"}}, nil, llm.StreamCallbacks{}); err == nil {
		t.Fatal("timeout failed open")
	}
}

func TestConfigValidation(t *testing.T) {
	for _, bad := range []Rule{{Kind: "bad-kind", Prefix: "test"}, {Kind: "CUSTOM", Pattern: "("}, {Kind: "CUSTOM", Pattern: ".*"}} {
		c := DefaultConfig()
		c.Rules = []Rule{bad}
		if c.Validate() == nil {
			t.Fatalf("accepted invalid rule %+v", bad)
		}
	}
}
