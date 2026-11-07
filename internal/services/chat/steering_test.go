package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	llm "slimebot/internal/services/llm"
)

func TestSteeringIdentityPersistenceAndSeal(t *testing.T) {
	repo := newTestRepo(t)
	session, err := repo.CreateSession(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	s := NewChatService(repo, nil, nil, nil, nil)
	ctx, finish, err := s.beginTurn(context.Background(), session.ID, "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	ctx.Value(turnInboxKey{}).(*turnInbox).open()
	if _, _, err = s.beginTurn(ctx, session.ID, "turn-2"); !errors.Is(err, ErrTurnBusy) {
		t.Fatal("concurrent session turn accepted")
	}
	if _, err = s.Steer(ctx, session.ID, "old", "input-1", "change"); !errors.Is(err, ErrStaleTurn) {
		t.Fatal("stale turn accepted")
	}
	first, err := s.Steer(ctx, session.ID, "turn-1", "input-1", "change")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Steer(ctx, session.ID, "turn-1", "input-1", "change")
	if err != nil || again.MessageID != first.MessageID {
		t.Fatal("retry was not idempotent")
	}
	if _, err = s.Steer(ctx, session.ID, "turn-1", "input-1", "different"); err == nil {
		t.Fatal("changed retry accepted")
	}
	inbox := ctx.Value(turnInboxKey{}).(*turnInbox)
	if items := inbox.drain(true); len(items) != 1 || items[0].Content != "change" {
		t.Fatal(items)
	}
	inbox.drain(true)
	if _, err = s.Steer(ctx, session.ID, "turn-1", "input-2", "late"); !errors.Is(err, ErrStaleTurn) {
		t.Fatal("sealed turn accepted input")
	}
	messages, _, err := repo.ListSessionMessagesPage(ctx, session.ID, 20, nil, nil, nil, nil)
	if err != nil || len(messages) != 1 || messages[0].Content != "change" {
		t.Fatalf("saved messages: %v %v", messages, err)
	}
	finish()
	_, endNext, err := s.beginTurn(ctx, session.ID, "turn-2")
	if err != nil {
		t.Fatal(err)
	}
	defer endNext()
	if _, err = s.Steer(ctx, session.ID, "turn-1", "input-3", "wrong turn"); !errors.Is(err, ErrStaleTurn) {
		t.Fatal("old turn steered new turn")
	}
}

type steeringProvider struct {
	fakeThinkingProvider
	first  func(context.Context, llm.StreamCallbacks) (*llm.StreamResult, error)
	calls  int
	latest []llm.ChatMessage
}

func (p *steeringProvider) StreamChatWithTools(ctx context.Context, _ llm.ModelRuntimeConfig, messages []llm.ChatMessage, _ []llm.ToolDef, cb llm.StreamCallbacks) (*llm.StreamResult, error) {
	p.calls++
	p.latest = append([]llm.ChatMessage{}, messages...)
	if p.calls == 1 {
		return p.first(ctx, cb)
	}
	if cb.OnChunk != nil {
		if err := cb.OnChunk("followed new guidance"); err != nil {
			return nil, err
		}
	}
	return &llm.StreamResult{Type: llm.StreamResultText}, nil
}

func TestSteeringSkipsOutdatedToolsAndContinuesText(t *testing.T) {
	for _, tool := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "tool"}[tool], func(t *testing.T) {
			repo := newTestRepo(t)
			session, _ := repo.CreateSession(context.Background(), "test")
			started, release := make(chan struct{}), make(chan struct{})
			p := &steeringProvider{first: func(ctx context.Context, cb llm.StreamCallbacks) (*llm.StreamResult, error) {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				if tool {
					tc := llm.ToolCallInfo{ID: "outdated", Name: "file_write__write", Arguments: `{"path":"must-not-execute","content":"old direction"}`}
					return &llm.StreamResult{Type: llm.StreamResultToolCalls, ToolCalls: []llm.ToolCallInfo{tc}, AssistantMessage: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCallInfo{tc}}}, nil
				}
				_ = cb.OnChunk("original thought")
				return &llm.StreamResult{Type: llm.StreamResultText}, nil
			}}
			s := NewChatService(repo, nil, llm.NewFactory(p), nil, nil)
			ctx, end, _ := s.beginTurn(context.Background(), session.ID, "turn")
			defer end()
			ctx.Value(turnInboxKey{}).(*turnInbox).open()
			done := make(chan error, 1)
			go func() {
				_, err := s.agent.RunAgentLoop(ctx, llm.ModelRuntimeConfig{Provider: llm.ProviderOpenAI}, session.ID, []llm.ChatMessage{{Role: "user", Content: "original task"}}, nil, map[string]struct{}{}, AgentCallbacks{OnChunk: func(string) error { return nil }}, AgentLoopOptions{steering: ctx.Value(turnInboxKey{}).(*turnInbox)})
				done <- err
			}()
			<-started
			if _, err := s.Steer(ctx, session.ID, "turn", "input", "only explain, do not write files"); err != nil {
				t.Fatal(err)
			}
			close(release)
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("turn did not continue")
			}
			if p.calls != 2 || p.latest[len(p.latest)-1].Content != "only explain, do not write files" {
				t.Fatal("guidance did not reach next request")
			}
			if tool {
				found := false
				for _, msg := range p.latest {
					if msg.Role == "tool" && strings.Contains(msg.Content, "Skipped") {
						found = true
					}
				}
				if !found {
					t.Fatal("outdated tool was not resolved before new user input")
				}
			}
		})
	}
}

func TestSteeringInterruptsApprovalWait(t *testing.T) {
	s := NewChatService(newTestRepo(t), nil, nil, nil, nil)
	ctx, end, _ := s.beginTurn(context.Background(), "session", "turn")
	defer end()
	inbox := ctx.Value(turnInboxKey{}).(*turnInbox)
	waiting, cancel := inbox.approvalContext(ctx)
	defer cancel()
	inbox.mu.Lock()
	close(inbox.changed)
	inbox.pending = append(inbox.pending, SteeringMessage{Content: "change"})
	inbox.mu.Unlock()
	select {
	case <-waiting.Done():
	case <-time.After(time.Second):
		t.Fatal("approval was left waiting on superseded action")
	}
}
