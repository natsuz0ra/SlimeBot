package chat

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"slimebot/internal/domain"
	llm "slimebot/internal/services/llm"
)

func TestForegroundTurnPreemptsIdleCompaction(t *testing.T) {
	service := &ChatService{turns: map[string]bool{}}
	idleCtx, releaseIdle, err := service.acquireIdleTurn(context.Background(), "session")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	type acquisition struct {
		release func()
		err     error
	}
	acquired := make(chan acquisition, 1)
	go func() {
		release, err := service.acquireTurnWithContext(ctx, "session")
		acquired <- acquisition{release: release, err: err}
	}()

	select {
	case <-idleCtx.Done():
	case <-ctx.Done():
		t.Fatal("foreground turn did not cancel idle compaction")
	}
	select {
	case result := <-acquired:
		if result.release != nil {
			result.release()
		}
		t.Fatalf("foreground turn acquired the lease before idle cleanup: %v", result.err)
	default:
	}
	releaseIdle()
	select {
	case result := <-acquired:
		if result.err != nil || result.release == nil {
			t.Fatalf("foreground turn did not acquire the lease: %v", result.err)
		}
		result.release()
	case <-ctx.Done():
		t.Fatal("foreground turn remained blocked after idle cleanup")
	}
}

func TestBackgroundStartRejectsWorkAfterCancellation(t *testing.T) {
	service := &ChatService{}
	runtime := service.backgrounds()
	runtime.cancel()
	called := make(chan struct{}, 1)
	if runtime.start(func() { called <- struct{}{} }) {
		t.Fatal("background work was accepted after cancellation")
	}
	select {
	case <-called:
		t.Fatal("cancelled background runtime started a goroutine")
	default:
	}
}

func TestCompactContextReportsNoOpWhenThereIsNoCompactableRegion(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "short manual compaction")
	if err != nil {
		t.Fatal(err)
	}
	model, err := repo.CreateLLMConfig(ctx, domain.LLMConfig{
		Name: "fake", Provider: llm.ProviderOpenAI, BaseURL: "http://fake", APIKey: "key",
		Model: "fake", ContextSize: 32768,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{SessionID: session.ID, Role: "user", Content: "short request"}); err != nil {
		t.Fatal(err)
	}
	service := NewChatService(repo, nil, llm.NewFactory(&overflowProvider{}), nil, nil)
	result, err := service.CompactContext(ctx, session.ID, model.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Compacted {
		t.Fatal("no-op manual compaction reported a committed checkpoint")
	}
	snapshot, err := repo.GetContextSnapshot(ctx, session.ID)
	if err != nil || len(snapshot.Checkpoints) != 0 {
		t.Fatalf("no-op manual compaction created checkpoints: %#v err=%v", snapshot.Checkpoints, err)
	}
}

func TestCompactContextCommitsManualCheckpointWithoutFabricatingUserInput(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "manual compaction")
	if err != nil {
		t.Fatal(err)
	}
	model, err := repo.CreateLLMConfig(ctx, domain.LLMConfig{
		Name: "fake", Provider: llm.ProviderOpenAI, BaseURL: "http://fake", APIKey: "key",
		Model: "fake", ContextSize: 32768,
	})
	if err != nil {
		t.Fatal(err)
	}
	latestUser := "keep the real latest user request"
	for i := 0; i < 8; i++ {
		role := "user"
		content := fmt.Sprintf("historical request %d %s", i, strings.Repeat("old context ", 500))
		if i%2 == 1 {
			role = "assistant"
		}
		if i == 7 {
			role = "user"
			content = latestUser
		}
		if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{SessionID: session.ID, Role: role, Content: content}); err != nil {
			t.Fatal(err)
		}
	}
	provider := &overflowProvider{}
	service := NewChatService(repo, nil, llm.NewFactory(provider), nil, nil)
	result, err := service.CompactContext(ctx, session.ID, model.ID)
	if err != nil || !result.IsCompacted || !result.Compacted {
		t.Fatalf("manual compaction failed: result=%+v err=%v", result, err)
	}
	snapshot, err := repo.GetContextSnapshot(ctx, session.ID)
	if err != nil || len(snapshot.Checkpoints) != 1 || snapshot.Checkpoints[0].Reason != "manual" {
		t.Fatalf("manual checkpoint missing: %#v err=%v", snapshot.Checkpoints, err)
	}
	var users []string
	for _, entry := range snapshot.Entries {
		message, err := entry.Message()
		if err != nil {
			t.Fatal(err)
		}
		if message.Role == "user" {
			users = append(users, message.Content)
		}
	}
	if len(users) != 5 || users[len(users)-1] != latestUser {
		t.Fatalf("compaction fabricated or lost user messages: %#v", users)
	}
	messages, err := repo.ListAllSessionMessages(ctx, session.ID, -1)
	if err != nil || len(messages) != 8 {
		t.Fatalf("compaction changed UI history: messages=%d err=%v", len(messages), err)
	}
}
