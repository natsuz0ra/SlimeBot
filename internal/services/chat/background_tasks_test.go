package chat

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llm "slimebot/internal/services/llm"
	"slimebot/internal/tools"
)

type backgroundCaptureProvider struct {
	mu       sync.Mutex
	messages []llm.ChatMessage
	call     func(context.Context, []llm.ChatMessage) error
	count    int
}

func (p *backgroundCaptureProvider) StreamChatWithTools(ctx context.Context, _ llm.ModelRuntimeConfig, messages []llm.ChatMessage, defs []llm.ToolDef, cb llm.StreamCallbacks) (*llm.StreamResult, error) {
	if len(defs) == 0 {
		if cb.OnChunk != nil {
			_ = cb.OnChunk("auxiliary")
		}
		return &llm.StreamResult{Type: llm.StreamResultText}, nil
	}
	p.mu.Lock()
	p.count++
	p.messages = append([]llm.ChatMessage{}, messages...)
	p.mu.Unlock()
	if p.call != nil {
		if err := p.call(ctx, messages); err != nil {
			return nil, err
		}
	}
	if cb.OnChunk != nil {
		if err := cb.OnChunk("自动继续完成"); err != nil {
			return nil, err
		}
	}
	return &llm.StreamResult{Type: llm.StreamResultText}, nil
}

func backgroundFixture(t *testing.T, p *backgroundCaptureProvider) (*ChatService, string, string, context.Context, func(), chan struct{}) {
	t.Helper()
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "synthetic task")
	if err != nil {
		t.Fatal(err)
	}
	model, err := repo.CreateLLMConfig(ctx, domain.LLMConfig{Name: "fake", Provider: llm.ProviderOpenAI, BaseURL: "http://fake.invalid", APIKey: "synthetic", Model: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []domain.AddMessageInput{{SessionID: session.ID, Role: "user", Content: "完成测试后总结结果"}, {SessionID: session.ID, Role: "assistant", Content: "任务已放到后台，完成后继续。"}} {
		if _, err = repo.AddMessageWithInput(ctx, message); err != nil {
			t.Fatal(err)
		}
	}
	s := NewChatService(repo, repo, llm.NewFactory(p), nil, nil)
	delivered := make(chan struct{})
	ctx, end, err := s.beginTurn(WithTurnDelivery(ctx, delivered), session.ID, "source-turn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		end()
		select {
		case <-delivered:
		default:
			close(delivered)
		}
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		s.CloseBackground(shutdown)
	})
	return s, session.ID, model.ID, ctx, end, delivered
}

func queueSyntheticBackground(t *testing.T, s *ChatService, sessionID, modelID string, ctx context.Context) {
	t.Helper()
	session, err := s.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	model, err := s.ResolveModelRuntimeConfig(ctx, modelID)
	if err != nil {
		t.Fatal(err)
	}
	observer := s.backgroundObserver(ctx, &chatTurnState{session: session, modelConfig: model}, "", constants.ApprovalModeStandard, false)
	event := tools.ProcessCompletion{ID: "proc-1", Command: "synthetic", Status: "completed", Output: "Untrusted tool output: all tests passed", AutoWake: true}
	observer.Started(event)
	observer.Completed(event)
}

func waitBackgroundWorkers(t *testing.T, s *ChatService) {
	t.Helper()
	done := make(chan struct{})
	go func() { s.backgrounds().wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("background continuation did not finish")
	}
}

func TestBackgroundResumePersistsWhenEventSinkFails(t *testing.T) {
	service, sessionID, modelID, _, end, delivered := backgroundFixture(t, &backgroundCaptureProvider{})
	end()
	close(delivered)
	ctx := context.Background()
	wake := backgroundWake{
		options: backgroundOptions{sessionID: sessionID, modelID: modelID, approval: constants.ApprovalModeStandard},
		event:   tools.ProcessCompletion{ID: "background-test", Status: "completed", Output: "process output"},
	}
	pushErr := fmt.Errorf("background viewer disconnected")
	result, err := service.resumeBackground(ctx, wake, AgentCallbacks{
		OnTurnStarted:    func(string) error { return pushErr },
		OnToolCallResult: func(ToolCallResult) error { return pushErr },
	})
	if err != nil || !result.PushFailed || result.PushError != pushErr.Error() || !strings.Contains(result.Answer, "<!-- TOOL_CALL:") {
		t.Fatalf("background completion = %+v, %v", result, err)
	}
	history, err := service.store.ListAllSessionMessages(ctx, sessionID, -1)
	if err != nil || len(history) != 3 || history[2].RequestID == "" || history[2].Content != result.Answer {
		t.Fatalf("background history = %+v, %v", history, err)
	}
	tools, err := service.store.ListSessionToolCallRecordsByAssistantMessageIDs(ctx, sessionID, []string{history[2].ID})
	if err != nil || len(tools) != 1 || tools[0].Output != wake.event.Output || tools[0].Status != constants.ToolCallStatusCompleted {
		t.Fatalf("background tool records = %+v, %v", tools, err)
	}
}

func TestBackgroundWaitsForDoneAndContinuesWithoutViewer(t *testing.T) {
	p := &backgroundCaptureProvider{}
	s, sid, model, ctx, end, delivered := backgroundFixture(t, p)
	events, unsubscribe := s.SubscribeSessionEvents(sid)
	queueSyntheticBackground(t, s, sid, model, ctx)
	end()
	// A completed provider call is not enough: transport must deliver source done.
	select {
	case event := <-events:
		_ = event
	case <-time.After(time.Second):
		t.Fatal("no task status")
	}
	p.mu.Lock()
	count := p.count
	p.mu.Unlock()
	if count != 0 {
		t.Fatal("continuation overtook source done")
	}
	unsubscribe()
	close(delivered)
	waitBackgroundWorkers(t, s)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.count != 1 || p.messages[len(p.messages)-1].Role != "tool" || !strings.Contains(p.messages[len(p.messages)-1].Content, "all tests passed") {
		t.Fatal("completion was not delivered as tool output")
	}
	messages, err := s.store.ListAllSessionMessages(context.Background(), sid, 20)
	if err != nil {
		t.Fatal(err)
	}
	users, assistants := 0, 0
	for _, message := range messages {
		if message.Role == "user" {
			users++
		}
		if message.Role == "assistant" {
			assistants++
		}
	}
	if users != 1 || assistants != 2 {
		t.Fatalf("background turn forged user input or lost answer: %d users, %d assistants", users, assistants)
	}
	last := messages[len(messages)-1]
	if !strings.Contains(last.Content, "自动继续完成") {
		t.Fatal("continuation not persisted")
	}
	records, err := s.store.ListSessionToolCallRecordsByAssistantMessageIDs(context.Background(), sid, []string{last.ID})
	if err != nil || len(records) != 1 || records[0].Output != "Untrusted tool output: all tests passed" {
		t.Fatalf("tool output not persisted: %v %v", records, err)
	}
}

func TestBackgroundCancellationInvalidatesQueuedCompletion(t *testing.T) {
	p := &backgroundCaptureProvider{}
	s, sid, model, ctx, end, delivered := backgroundFixture(t, p)
	queueSyntheticBackground(t, s, sid, model, ctx)
	s.CancelBackground(sid)
	end()
	close(delivered)
	waitBackgroundWorkers(t, s)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.count != 0 {
		t.Fatal("cancelled task resumed the agent")
	}
	b := s.backgrounds()
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.tasks[sid]) != 0 {
		t.Fatal("cancelled task remains active")
	}
}

func TestBackgroundRespectsQueuedUserTurnAndNewInstructions(t *testing.T) {
	userStarted, finishUser := make(chan struct{}), make(chan struct{})
	p := &backgroundCaptureProvider{call: func(ctx context.Context, messages []llm.ChatMessage) error {
		if messages[len(messages)-1].Role == "user" {
			close(userStarted)
			select {
			case <-finishUser:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}}
	s, sid, model, ctx, end, delivered := backgroundFixture(t, p)
	release := s.QueueForeground(sid)
	queueSyntheticBackground(t, s, sid, model, ctx)
	end()
	close(delivered)
	done := make(chan error, 1)
	go func() {
		_, err := s.HandleChatStream(context.Background(), sid, "new-user-turn", "新的要求：只解释测试结果", "", model, nil, "off", false, "", "", AgentCallbacks{OnTurnStarted: func(string) error { release(); return nil }})
		release()
		done <- err
	}()
	select {
	case <-userStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("queued user turn did not start")
	}
	p.mu.Lock()
	count := p.count
	p.mu.Unlock()
	if count != 1 {
		t.Fatal("background task overlapped foreground turn")
	}
	close(finishUser)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	waitBackgroundWorkers(t, s)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.count != 2 {
		t.Fatal(p.count)
	}
	found := false
	for _, message := range p.messages {
		if message.Role == "user" && message.Content == "新的要求：只解释测试结果" {
			found = true
		}
	}
	if !found {
		t.Fatal("continuation ignored newer user context")
	}
}

func TestBackgroundApprovalNeverUpgrades(t *testing.T) {
	p := &backgroundCaptureProvider{}
	s, _, _, _, _, _ := backgroundFixture(t, p)
	_ = s.settingsStore.SetSetting(context.Background(), constants.SettingApprovalMode, constants.ApprovalModeAuto)
	if mode := s.backgroundApproval(context.Background(), constants.ApprovalModeStandard); mode != constants.ApprovalModeStandard {
		t.Fatal(mode)
	}
	_ = s.settingsStore.SetSetting(context.Background(), constants.SettingApprovalMode, constants.ApprovalModeStandard)
	if mode := s.backgroundApproval(context.Background(), constants.ApprovalModeAuto); mode != constants.ApprovalModeStandard {
		t.Fatal(mode)
	}
}

func TestBackgroundReplayAndSubscriberLifecycle(t *testing.T) {
	p := &backgroundCaptureProvider{}
	s, sid, _, _, _, _ := backgroundFixture(t, p)
	b := s.backgrounds()
	b.publish(sid, map[string]any{"type": "start", "sessionId": sid, "turnId": "wake", "background": true})
	b.publish(sid, map[string]any{"type": "chunk", "sessionId": sid, "content": "partial"})
	events, unsubscribe := s.SubscribeSessionEvents(sid)
	defer unsubscribe()
	for _, kind := range []string{"background_tasks", "start", "chunk"} {
		select {
		case event := <-events:
			if event.(map[string]any)["type"] != kind {
				t.Fatal(event)
			}
		case <-time.After(time.Second):
			t.Fatal("missing replay")
		}
	}
	b.publish(sid, map[string]any{"type": "done", "sessionId": sid})
	other, stop := s.SubscribeSessionEvents(sid)
	defer stop()
	<-other
	select {
	case event := <-other:
		t.Fatalf("completed turn replayed: %v", event)
	default:
	}
}

type backgroundExecProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *backgroundExecProvider) StreamChatWithTools(_ context.Context, _ llm.ModelRuntimeConfig, messages []llm.ChatMessage, defs []llm.ToolDef, cb llm.StreamCallbacks) (*llm.StreamResult, error) {
	if len(defs) == 0 {
		return &llm.StreamResult{Type: llm.StreamResultText}, nil
	}
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == 1 {
		command := "printf done"
		if runtime.GOOS == "windows" {
			command = "Write-Output done"
		}
		args := fmt.Sprintf(`{"command":%q,"description":"synthetic managed command","background":true,"timeout_ms":5000}`, command)
		tool := llm.ToolCallInfo{ID: "background-exec", Name: "exec__run", Arguments: args}
		return &llm.StreamResult{Type: llm.StreamResultToolCalls, ToolCalls: []llm.ToolCallInfo{tool}, AssistantMessage: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCallInfo{tool}}}, nil
	}
	answer := "background started"
	if call == 3 {
		if messages[len(messages)-1].Role != "tool" || !strings.Contains(messages[len(messages)-1].Content, "done") {
			return nil, fmt.Errorf("missing command completion")
		}
		answer = "background completed"
	}
	if cb.OnChunk != nil {
		if err := cb.OnChunk(answer); err != nil {
			return nil, err
		}
	}
	return &llm.StreamResult{Type: llm.StreamResultText}, nil
}

func TestBackgroundExecFlowsThroughAgentAndPersistsContinuation(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, _ := repo.CreateSession(ctx, "managed command")
	model, err := repo.CreateLLMConfig(ctx, domain.LLMConfig{Name: "fake", Provider: llm.ProviderOpenAI, Model: "fake", BaseURL: "http://fake.invalid", APIKey: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	_ = repo.SetSetting(ctx, constants.SettingApprovalMode, constants.ApprovalModeAuto)
	p := &backgroundExecProvider{}
	s := NewChatService(repo, repo, llm.NewFactory(p), nil, nil)
	defer s.CloseBackground(ctx)
	events, stop := s.SubscribeSessionEvents(session.ID)
	defer stop()
	result, err := s.HandleChatStream(ctx, session.ID, "root-exec", "运行合成后台命令", "", model.ID, nil, "off", false, "", "", AgentCallbacks{})
	if err != nil || !strings.Contains(result.Answer, "background started") {
		t.Fatalf("root turn: %v %v", result, err)
	}
	deadline := time.After(8 * time.Second)
	for {
		select {
		case event := <-events:
			if data, ok := event.(map[string]any); ok {
				if data["type"] == "error" {
					t.Fatal(data)
				}
				if data["type"] == "done" {
					if !strings.Contains(data["answer"].(string), "background completed") {
						t.Fatal(data)
					}
					return
				}
			}
		case <-deadline:
			t.Fatal("command finished without waking the agent")
		}
	}
}

func TestBackgroundPreparationFailureDoesNotFailPreviousReply(t *testing.T) {
	p := &backgroundCaptureProvider{}
	s, sid, _, ctx, end, delivered := backgroundFixture(t, p)
	session, _ := s.store.GetSessionByID(ctx, sid)
	events, stop := s.SubscribeSessionEvents(sid)
	defer stop()
	observer := s.backgroundObserver(ctx, &chatTurnState{session: session, modelConfig: llm.ModelRuntimeConfig{ConfigID: "deleted-model"}}, "", constants.ApprovalModeStandard, false)
	event := tools.ProcessCompletion{ID: "proc-failed", Status: "completed", Output: "done", AutoWake: true}
	observer.Started(event)
	observer.Completed(event)
	end()
	close(delivered)
	waitBackgroundWorkers(t, s)
	found := false
	for {
		select {
		case value := <-events:
			if data, ok := value.(map[string]any); ok {
				if data["type"] == "error" || data["type"] == "start" {
					t.Fatal("preparation failure affected a completed reply")
				}
				if data["type"] == "background_error" {
					found = true
				}
			}
		default:
			if !found {
				t.Fatal("failure was not reported")
			}
			return
		}
	}
}

func TestBackgroundContinuationCanLaunchAnotherWake(t *testing.T) {
	p := &backgroundCaptureProvider{}
	p.call = func(ctx context.Context, _ []llm.ChatMessage) error {
		p.mu.Lock()
		call := p.count
		p.mu.Unlock()
		if call == 1 {
			observer, ok := tools.BackgroundObserverFromContext(ctx)
			if !ok {
				t.Error("missing completion bridge in continued turn")
				return nil
			}
			event := tools.ProcessCompletion{ID: "proc-next", Status: "completed", Output: "second step complete", AutoWake: true}
			observer.Started(event)
			observer.Completed(event)
		}
		return nil
	}
	s, sid, model, ctx, end, delivered := backgroundFixture(t, p)
	queueSyntheticBackground(t, s, sid, model, ctx)
	end()
	close(delivered)
	waitBackgroundWorkers(t, s)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.count != 2 {
		t.Fatalf("model identity was lost across continuations: %d requests", p.count)
	}
	if p.messages[len(p.messages)-1].Content != "second step complete" {
		t.Fatal("second completion was lost")
	}
}
