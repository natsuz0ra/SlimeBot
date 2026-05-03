package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"
)

func TestBuildContextMessages_SystemPrefixStableAndNoLocalDateTime(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewChatService(repo, nil, nil, nil, nil)
	ctx := context.Background()

	msgs1, err := svc.BuildContextMessages(ctx, "session-1", llmsvc.ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("BuildContextMessages first call failed: %v", err)
	}
	if len(msgs1) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs1))
	}
	firstSystem1 := msgs1[0]
	if firstSystem1.Role != "system" {
		t.Fatalf("expected first message role system, got %q", firstSystem1.Role)
	}
	if strings.Contains(firstSystem1.Content, "## Runtime Environment") {
		t.Fatalf("first system prompt should not include runtime environment: %q", firstSystem1.Content)
	}
	if strings.Contains(firstSystem1.Content, "Local date:") || strings.Contains(firstSystem1.Content, "Local time:") {
		t.Fatalf("first system prompt should not include local date/time: %q", firstSystem1.Content)
	}

	time.Sleep(1200 * time.Millisecond)

	msgs2, err := svc.BuildContextMessages(ctx, "session-1", llmsvc.ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("BuildContextMessages second call failed: %v", err)
	}
	if len(msgs2) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs2))
	}
	firstSystem2 := msgs2[0]
	if firstSystem1.Content != firstSystem2.Content {
		t.Fatalf("expected stable first system prompt across calls")
	}

	runtimeSystem := msgs1[1]
	if runtimeSystem.Role != "system" {
		t.Fatalf("expected runtime message role system, got %q", runtimeSystem.Role)
	}
	if !strings.Contains(runtimeSystem.Content, "## Runtime Environment") {
		t.Fatalf("expected runtime environment message, got %q", runtimeSystem.Content)
	}
	if strings.Contains(runtimeSystem.Content, "Local date:") || strings.Contains(runtimeSystem.Content, "Local time:") {
		t.Fatalf("runtime message should not include local date/time: %q", runtimeSystem.Content)
	}
}

func TestBuildContextMessages_IncludesConfigDirInCLI(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewChatService(repo, nil, nil, nil, nil)
	svc.SetRunContext(RunContext{
		ConfigHomeDir:        "/home/user/.slimebot",
		ConfigDirDescription: "/home/user/.slimebot/\n  skills/\n  storage/\n",
		WorkingDir:           "/home/user/project",
		IsCLI:                true,
	})
	ctx := context.Background()

	msgs, err := svc.BuildContextMessages(ctx, "session-cli", llmsvc.ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Locate the runtime environment message
	found := false
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "Config directory") {
			found = true
			if !strings.Contains(m.Content, "/home/user/.slimebot") {
				t.Fatal("expected config home dir in runtime prompt")
			}
			if !strings.Contains(m.Content, "skills/") {
				t.Fatal("expected skills/ in directory listing")
			}
			if !strings.Contains(m.Content, "Current working directory: /home/user/project") {
				t.Fatal("expected working dir in CLI mode runtime prompt")
			}
		}
	}
	if !found {
		t.Fatal("expected runtime environment message with config directory")
	}
}

func TestBuildContextMessages_ServerMode_NoWorkingDir(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewChatService(repo, nil, nil, nil, nil)
	svc.SetRunContext(RunContext{
		ConfigHomeDir: "/home/user/.slimebot",
		IsCLI:         false,
	})
	ctx := context.Background()

	msgs, err := svc.BuildContextMessages(ctx, "session-srv", llmsvc.ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "Config directory") {
			if strings.Contains(m.Content, "Current working directory") {
				t.Fatal("server mode should not include working directory")
			}
		}
	}
}

func TestBuildContextMessages_NoRunContext_OmitsConfigDir(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewChatService(repo, nil, nil, nil, nil)
	// Do not set RunContext (zero value)
	ctx := context.Background()

	msgs, err := svc.BuildContextMessages(ctx, "session-norc", llmsvc.ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "## Runtime Environment") {
			if strings.Contains(m.Content, "Config directory") {
				t.Fatal("expected no config directory when RunContext is zero-valued")
			}
			if strings.Contains(m.Content, "Current working directory") {
				t.Fatal("expected no working directory when RunContext is zero-valued")
			}
		}
	}
}

func TestBuildContextMessages_IncludesFullHistoryBelowContextSize(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "history-rounds")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	for i := 0; i < 80; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{
			SessionID: session.ID,
			Role:      role,
			Content:   fmt.Sprintf("m-%d", i),
		}); err != nil {
			t.Fatalf("AddMessageWithInput failed at %d: %v", i, err)
		}
	}

	svc := NewChatService(repo, nil, nil, nil, nil)
	svc.SetContextHistoryRounds(20)
	msgs20, err := svc.BuildContextMessages(ctx, session.ID, llmsvc.ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("BuildContextMessages failed: %v", err)
	}
	if got := len(msgs20); got != 82 { // system(2) + full history(80)
		t.Fatalf("expected 82 messages below context threshold, got %d", got)
	}
}

func TestBuildContextUsageReportsActualContextBelowThreshold(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "usage")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	for i := 0; i < 4; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{
			SessionID: session.ID,
			Role:      role,
			Content:   fmt.Sprintf("message-%d", i),
		}); err != nil {
			t.Fatalf("AddMessageWithInput failed at %d: %v", i, err)
		}
	}

	svc := NewChatService(repo, nil, nil, nil, nil)
	usage, err := svc.BuildContextUsage(ctx, session.ID, llmsvc.ModelRuntimeConfig{
		ConfigID:    "model-1",
		ContextSize: 128_000,
	})
	if err != nil {
		t.Fatalf("BuildContextUsage failed: %v", err)
	}
	if usage.SessionID != session.ID || usage.ModelConfigID != "model-1" {
		t.Fatalf("unexpected usage identity: %+v", usage)
	}
	if usage.UsedTokens <= 0 || usage.TotalTokens != 128_000 {
		t.Fatalf("unexpected token counts: %+v", usage)
	}
	if usage.UsedPercent <= 0 || usage.AvailablePercent >= 100 {
		t.Fatalf("unexpected percentages: %+v", usage)
	}
	if usage.IsCompacted {
		t.Fatalf("usage below threshold should not be compacted: %+v", usage)
	}
}

func TestBuildContextUsageReportsCompactedActualContextWhenExceeded(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "usage-compact")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	for i := 0; i < 12; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{
			SessionID: session.ID,
			Role:      role,
			Content:   fmt.Sprintf("message-%02d %s", i, strings.Repeat("长上下文", 220)),
		}); err != nil {
			t.Fatalf("AddMessageWithInput failed at %d: %v", i, err)
		}
	}

	provider := &compactSummaryProvider{summary: "压缩摘要"}
	svc := NewChatService(repo, nil, llmsvc.NewFactory(provider), nil, nil)
	svc.SetContextHistoryRounds(2)
	usage, err := svc.BuildContextUsage(ctx, session.ID, llmsvc.ModelRuntimeConfig{
		Provider:    llmsvc.ProviderOpenAI,
		BaseURL:     "http://fake",
		APIKey:      "key",
		Model:       "fake-model",
		ContextSize: 5_000,
	})
	if err != nil {
		t.Fatalf("BuildContextUsage failed: %v", err)
	}
	if provider.compactCalls != 1 {
		t.Fatalf("expected one compact call, got %d", provider.compactCalls)
	}
	if !usage.IsCompacted || usage.CompactedAt == "" {
		t.Fatalf("expected compacted usage with timestamp, got %+v", usage)
	}
	if usage.UsedTokens <= 0 || usage.UsedTokens >= usage.TotalTokens {
		t.Fatalf("expected actual compacted context below total, got %+v", usage)
	}
}

func TestBuildContextMessages_CompactsWhenContextSizeExceeded(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "compact")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	for i := 0; i < 12; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{
			SessionID: session.ID,
			Role:      role,
			Content:   fmt.Sprintf("message-%02d %s", i, strings.Repeat("长上下文", 20)),
		}); err != nil {
			t.Fatalf("AddMessageWithInput failed at %d: %v", i, err)
		}
	}

	provider := &compactSummaryProvider{summary: "这是压缩后的会话摘要"}
	svc := NewChatService(repo, nil, llmsvc.NewFactory(provider), nil, nil)
	svc.SetContextHistoryRounds(2)

	msgs, err := svc.BuildContextMessages(ctx, session.ID, llmsvc.ModelRuntimeConfig{
		Provider:    llmsvc.ProviderOpenAI,
		BaseURL:     "http://fake",
		APIKey:      "key",
		Model:       "fake-model",
		ContextSize: 80,
	})
	if err != nil {
		t.Fatalf("BuildContextMessages failed: %v", err)
	}

	if provider.compactCalls != 1 {
		t.Fatalf("expected one compact call, got %d", provider.compactCalls)
	}
	joined := joinChatMessageContent(msgs)
	if !strings.Contains(joined, "这是压缩后的会话摘要") {
		t.Fatalf("expected compact summary in context, got: %s", joined)
	}
	if strings.Contains(joined, "message-00") {
		t.Fatalf("old history should be summarized instead of replayed: %s", joined)
	}
	if !strings.Contains(joined, "message-10") || !strings.Contains(joined, "message-11") {
		t.Fatalf("recent tail should be preserved, got: %s", joined)
	}

	summary, err := repo.GetSessionContextSummary(ctx, session.ID, "")
	if err != nil {
		t.Fatalf("expected stored compact summary: %v", err)
	}
	if summary.SummarizedUntilSeq != 2 {
		t.Fatalf("expected summarizedUntilSeq=2, got %d", summary.SummarizedUntilSeq)
	}
}

func TestBuildContextMessages_ReusesExistingCompactSummary(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "compact-reuse")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	for i := 0; i < 8; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{
			SessionID: session.ID,
			Role:      role,
			Content:   fmt.Sprintf("message-%02d %s", i, strings.Repeat("上下文", 20)),
		}); err != nil {
			t.Fatalf("AddMessageWithInput failed at %d: %v", i, err)
		}
	}
	if err := repo.UpsertSessionContextSummary(ctx, &domain.SessionContextSummary{
		SessionID:               session.ID,
		ModelConfigID:           "",
		Summary:                 "已有压缩摘要",
		SummarizedUntilSeq:      4,
		PreCompactTokenEstimate: 100,
	}); err != nil {
		t.Fatalf("UpsertSessionContextSummary failed: %v", err)
	}

	provider := &compactSummaryProvider{summary: "不应重新生成"}
	svc := NewChatService(repo, nil, llmsvc.NewFactory(provider), nil, nil)
	svc.SetContextHistoryRounds(2)

	msgs, err := svc.BuildContextMessages(ctx, session.ID, llmsvc.ModelRuntimeConfig{
		Provider:    llmsvc.ProviderOpenAI,
		BaseURL:     "http://fake",
		APIKey:      "key",
		Model:       "fake-model",
		ContextSize: 30,
	})
	if err != nil {
		t.Fatalf("BuildContextMessages failed: %v", err)
	}

	if provider.compactCalls != 0 {
		t.Fatalf("expected existing summary to be reused, got %d compact calls", provider.compactCalls)
	}
	joined := joinChatMessageContent(msgs)
	if !strings.Contains(joined, "已有压缩摘要") {
		t.Fatalf("expected existing compact summary in context, got: %s", joined)
	}
	if strings.Contains(joined, "message-00") || strings.Contains(joined, "message-03") {
		t.Fatalf("summarized messages should not be replayed: %s", joined)
	}
}

func TestBuildContextMessages_RollsExistingSummaryForwardWithNewHistory(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "compact-roll-forward")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	for i := 0; i < 24; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{
			SessionID: session.ID,
			Role:      role,
			Content:   fmt.Sprintf("message-%02d %s", i, strings.Repeat("新增上下文", 20)),
		}); err != nil {
			t.Fatalf("AddMessageWithInput failed at %d: %v", i, err)
		}
	}
	if err := repo.UpsertSessionContextSummary(ctx, &domain.SessionContextSummary{
		SessionID:               session.ID,
		ModelConfigID:           "",
		Summary:                 "旧摘要内容",
		SummarizedUntilSeq:      4,
		PreCompactTokenEstimate: 100,
	}); err != nil {
		t.Fatalf("UpsertSessionContextSummary failed: %v", err)
	}

	provider := &compactSummaryProvider{summary: "替换后的完整摘要"}
	svc := NewChatService(repo, nil, llmsvc.NewFactory(provider), nil, nil)
	svc.SetContextHistoryRounds(2)

	msgs, err := svc.BuildContextMessages(ctx, session.ID, llmsvc.ModelRuntimeConfig{
		Provider:    llmsvc.ProviderOpenAI,
		BaseURL:     "http://fake",
		APIKey:      "key",
		Model:       "fake-model",
		ContextSize: 30,
	})
	if err != nil {
		t.Fatalf("BuildContextMessages failed: %v", err)
	}

	if provider.compactCalls != 1 {
		t.Fatalf("expected one compact call, got %d", provider.compactCalls)
	}
	if !strings.Contains(provider.lastPrompt, "已有摘要") || !strings.Contains(provider.lastPrompt, "旧摘要内容") {
		t.Fatalf("expected prior summary in compact prompt, got: %s", provider.lastPrompt)
	}
	if strings.Contains(provider.lastPrompt, "message-00") || strings.Contains(provider.lastPrompt, "message-03") {
		t.Fatalf("already summarized messages should not be re-summarized, got: %s", provider.lastPrompt)
	}
	if !strings.Contains(provider.lastPrompt, "message-04") || !strings.Contains(provider.lastPrompt, "message-13") {
		t.Fatalf("new messages after previous summary should be compacted, got: %s", provider.lastPrompt)
	}
	if strings.Contains(provider.lastPrompt, "message-14") || strings.Contains(provider.lastPrompt, "message-23") {
		t.Fatalf("recent tail should stay outside compact prompt, got: %s", provider.lastPrompt)
	}

	joined := joinChatMessageContent(msgs)
	if !strings.Contains(joined, "替换后的完整摘要") {
		t.Fatalf("expected replacement summary in context, got: %s", joined)
	}
	if strings.Contains(joined, "旧摘要内容\n替换后的完整摘要") {
		t.Fatalf("summary should be replaced, not appended: %s", joined)
	}
	if !strings.Contains(joined, "message-14") || !strings.Contains(joined, "message-23") {
		t.Fatalf("recent tail should be preserved, got: %s", joined)
	}

	summary, err := repo.GetSessionContextSummary(ctx, session.ID, "")
	if err != nil {
		t.Fatalf("expected stored compact summary: %v", err)
	}
	if summary.Summary != "替换后的完整摘要" {
		t.Fatalf("expected stored summary to be replaced, got %q", summary.Summary)
	}
	if summary.SummarizedUntilSeq != 14 {
		t.Fatalf("expected summarizedUntilSeq=14, got %d", summary.SummarizedUntilSeq)
	}
}

func TestBuildContextMessages_FallsBackToExistingSummaryAndRecentTailWhenRollForwardFails(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "compact-roll-forward-fail")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	for i := 0; i < 24; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{
			SessionID: session.ID,
			Role:      role,
			Content:   fmt.Sprintf("message-%02d %s", i, strings.Repeat("新增上下文", 20)),
		}); err != nil {
			t.Fatalf("AddMessageWithInput failed at %d: %v", i, err)
		}
	}
	if err := repo.UpsertSessionContextSummary(ctx, &domain.SessionContextSummary{
		SessionID:               session.ID,
		ModelConfigID:           "",
		Summary:                 "旧摘要内容",
		SummarizedUntilSeq:      4,
		PreCompactTokenEstimate: 100,
	}); err != nil {
		t.Fatalf("UpsertSessionContextSummary failed: %v", err)
	}

	provider := &compactSummaryProvider{err: errors.New("compact failed")}
	svc := NewChatService(repo, nil, llmsvc.NewFactory(provider), nil, nil)
	svc.SetContextHistoryRounds(2)

	msgs, err := svc.BuildContextMessages(ctx, session.ID, llmsvc.ModelRuntimeConfig{
		Provider:    llmsvc.ProviderOpenAI,
		BaseURL:     "http://fake",
		APIKey:      "key",
		Model:       "fake-model",
		ContextSize: 30,
	})
	if err != nil {
		t.Fatalf("BuildContextMessages failed: %v", err)
	}

	if provider.compactCalls != 1 {
		t.Fatalf("expected one compact call, got %d", provider.compactCalls)
	}
	joined := joinChatMessageContent(msgs)
	if !strings.Contains(joined, "旧摘要内容") {
		t.Fatalf("expected old summary fallback in context, got: %s", joined)
	}
	if strings.Contains(joined, "message-04") || strings.Contains(joined, "message-13") {
		t.Fatalf("failed roll-forward should not include oversized middle history, got: %s", joined)
	}
	if !strings.Contains(joined, "message-14") || !strings.Contains(joined, "message-23") {
		t.Fatalf("failed roll-forward should preserve recent tail, got: %s", joined)
	}

	summary, err := repo.GetSessionContextSummary(ctx, session.ID, "")
	if err != nil {
		t.Fatalf("expected stored compact summary: %v", err)
	}
	if summary.Summary != "旧摘要内容" {
		t.Fatalf("expected stored summary to remain unchanged, got %q", summary.Summary)
	}
	if summary.SummarizedUntilSeq != 4 {
		t.Fatalf("expected summarizedUntilSeq to remain 4, got %d", summary.SummarizedUntilSeq)
	}
}

func joinChatMessageContent(msgs []llmsvc.ChatMessage) string {
	var parts []string
	for _, msg := range msgs {
		parts = append(parts, msg.Content)
	}
	return strings.Join(parts, "\n")
}

type compactSummaryProvider struct {
	summary      string
	compactCalls int
	lastPrompt   string
	err          error
}

func (p *compactSummaryProvider) StreamChatWithTools(
	_ context.Context,
	_ llmsvc.ModelRuntimeConfig,
	messages []llmsvc.ChatMessage,
	toolDefs []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	if len(toolDefs) != 0 {
		return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
	}
	if len(messages) > 0 && strings.Contains(messages[len(messages)-1].Content, "压缩总结") {
		p.compactCalls++
		p.lastPrompt = messages[len(messages)-1].Content
		if p.err != nil {
			return nil, p.err
		}
		if callbacks.OnChunk != nil {
			if err := callbacks.OnChunk(p.summary); err != nil {
				return nil, err
			}
		}
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}
