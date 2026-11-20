package privacy

import (
	"context"
	"strings"
	"sync"
	"testing"

	"slimebot/internal/services/llm"
)

func TestRulesAndExceptions(t *testing.T) {
	s, _, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	c.Kinds["IBAN"] = true
	c.Words = []string{"Project-Rain"}
	c.IgnoreCase = true
	c.WholeWords = true
	c.Rules = []Rule{{Kind: "COMPANY", Prefix: "company_key_"}}
	for _, sample := range []struct{ input, kind string }{
		{"api_key = sk-proj-AbCd1234efgh5678ijkl9012", "API_KEY"},
		{"密码是 MySecret123", "PASSWORD"},
		{"postgres://user:Pass654321@db.local/app", "PASSWORD"},
		{"reach me at alice@private-company.com", "EMAIL"},
		{"电话 13800138000", "PHONE"},
		{"ID 110105194912310038", "ID_CARD"},
		{"card 4111 1111 1111 1111", "BANK_CARD"},
		{"IBAN GB82 WEST 1234 5698 7654 32", "IBAN"},
		{"a company_key_AbCd123456 value", "COMPANY"},
		{"Use project-rain today", "TERM"},
	} {
		tx, _ := s.begin(ctx, c, true)
		out, err := tx.maskText(ctx, sample.input, "")
		if err != nil {
			t.Fatal(err)
		}
		if out == sample.input || !strings.Contains(out, "SB_"+sample.kind+"_") {
			t.Fatalf("missing %s: %q", sample.kind, out)
		}
		if restored, err := tx.restore(out); err != nil || restored != sample.input {
			t.Fatal("rule roundtrip failed")
		}
	}
	for _, input := range []string{"alice@example.com", "alice@sample.test", "ID 110105194912310039", "card 4111111111111112", "PREFIXproject-rainSuffix"} {
		tx, _ := s.begin(ctx, c, true)
		out, err := tx.maskText(ctx, input, "")
		if err != nil || out != input {
			t.Fatalf("false positive: %q -> %q (%v)", input, out, err)
		}
	}
	c.Allow = []string{"MySecret123"}
	tx, _ := s.begin(ctx, c, true)
	if out, _ := tx.maskText(ctx, "密码是 MySecret123", ""); out != "密码是 MySecret123" {
		t.Fatal("allow list did not override masking")
	}
}

func TestSignedOriginalCannotLeakAfterEnabling(t *testing.T) {
	s, _, ctx := setup(t)
	called := false
	next := fakeProvider{run: func(context.Context, llm.ModelRuntimeConfig, []llm.ChatMessage, []llm.ToolDef, llm.StreamCallbacks) (*llm.StreamResult, error) {
		called = true
		return &llm.StreamResult{}, nil
	}}
	_, err := s.Wrap(next).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{}, []llm.ChatMessage{{Role: "user", Content: "DB_PASSWORD=EarlierSecret123"}, {Role: "assistant", ThinkingBlocks: []llm.ThinkingBlockInfo{{Thinking: "EarlierSecret123 is the password", Signature: "old-signed-value"}}}}, nil, llm.StreamCallbacks{})
	if err == nil || called {
		t.Fatal("protected value in old signature was sent")
	}
}

func TestConcurrentSessionsAndActiveMapping(t *testing.T) {
	s, _, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	active, _ := s.begin(ctx, c, false)
	masked, _ := active.maskText(ctx, "DB_PASSWORD=ActiveSecret123", "")
	active.finish(false)
	var wg sync.WaitGroup
	for i := 0; i < 150; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			scope := llm.WithPrivacyScope(ctx, strings.Repeat("x", i+1), "", "test")
			tx, err := s.begin(scope, c, false)
			if err != nil {
				t.Error(err)
				return
			}
			if _, err = tx.restore(masked); err == nil {
				t.Error("session boundary violated")
			}
			tx.finish(false)
		}(i)
	}
	wg.Wait()
	if len(s.scopes) > 128 {
		t.Fatal("unbounded scope cache")
	}
	if restored, err := active.restore(masked); err != nil || restored != "DB_PASSWORD=ActiveSecret123" {
		t.Fatal("eviction broke active mapping")
	}
}

func TestCollisionGetsAnotherTag(t *testing.T) {
	s, _, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	tx, _ := s.begin(ctx, c, true)
	p, _ := tx.placeholder("SECRET", "first-value")
	tx.values[p] = entry{"another-value", "SECRET"}
	next, err := tx.placeholder("SECRET", "first-value")
	if err != nil || p == next {
		t.Fatal("collision reused old mapping")
	}
}

func TestProtectionBeforeTruncation(t *testing.T) {
	s, _, ctx := setup(t)
	privateKey := "-----BEGIN PRIVATE KEY-----\n" + strings.Repeat("SyntheticBase64Material9876543210", 80) + "\n-----END PRIVATE KEY-----"
	protected, err := s.ProtectText(ctx, llm.ModelRuntimeConfig{}, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(protected, "SyntheticBase64Material") {
		t.Fatal("credential fragments exposed before truncation")
	}
	if !placeholderRE.MatchString(protected) {
		t.Fatal("private key was not protected")
	}
	c, _ := s.GetConfig(ctx)
	tx, _ := s.begin(ctx, c, false)
	if restored, err := tx.restore(protected); err != nil || restored != privateKey {
		t.Fatal("preparation did not retain local mapping")
	}
}

func TestLiteralValueCanResemblePlaceholder(t *testing.T) {
	s, _, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	tx, _ := s.begin(ctx, c, true)
	value := "SB_SECRET_0123456789abcdef"
	p, err := tx.placeholder("PASSWORD", value)
	if err != nil {
		t.Fatal(err)
	}
	if restored, err := tx.restore(p); err != nil || restored != value {
		t.Fatal("a literal restored value was treated as another placeholder")
	}
}

func TestOverlappingBlockCannotBeHiddenByAnotherRule(t *testing.T) {
	s, _, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	c.Words = []string{"Secret123"}
	c.Actions["TERM"] = Block
	_ = s.Update(ctx, c)
	preview, err := s.Preview(ctx, "DB_PASSWORD=Secret123")
	if err != nil || !preview.Blocked {
		t.Fatal("overlapping credential rule bypassed a custom-word block")
	}
	c.Allow = []string{"MySecret123"}
	_ = s.Update(ctx, c)
	preview, err = s.Preview(ctx, "DB_PASSWORD=MySecret123")
	if err != nil || preview.Blocked || preview.Text != "DB_PASSWORD=MySecret123" {
		t.Fatal("complete-value exception did not cover an overlapping word")
	}
	c.Kinds["TERM"] = false
	_ = s.Update(ctx, c)
	preview, err = s.Preview(ctx, "Secret123")
	if err != nil || preview.Blocked || preview.Text != "Secret123" {
		t.Fatal("disabled custom words still matched")
	}
}

func BenchmarkMaskRequest(b *testing.B) {
	for _, size := range []int{10000, 100000, 1000000} {
		b.Run(strings.TrimSpace(strings.Repeat("x", 0))+map[int]string{10000: "10KB", 100000: "100KB", 1000000: "1MB"}[size], func(b *testing.B) {
			store := &testStore{}
			service := NewService(store, b.TempDir())
			defer service.Close()
			config := DefaultConfig()
			config.Enabled = true
			_ = service.Update(context.Background(), config)
			input := strings.Repeat("safe text and source code. ", size/27) + " DB_PASSWORD=BenchmarkSecret123"
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				tx, err := service.begin(context.Background(), config, true)
				if err != nil {
					b.Fatal(err)
				}
				if _, err = tx.maskText(context.Background(), input, ""); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
