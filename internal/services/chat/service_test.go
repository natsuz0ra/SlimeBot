package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"
	plansvc "slimebot/internal/services/plan"
	prompts "slimebot/prompts"
)

func TestTitleStreamParser_ExtractsMemoryAndFiltersFromBody(t *testing.T) {
	parser := newTitleStreamParser(true)

	body := parser.Feed("<memory>{\"turn_summary\":\"用户偏好中文回复\",\"topic_hint\":\"回复偏好\"}</memory>\n正文第一段\n")
	if body != "正文第一段\n" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := parser.Memory(); got != "{\"turn_summary\":\"用户偏好中文回复\",\"topic_hint\":\"回复偏好\"}" {
		t.Fatalf("unexpected memory payload: %q", got)
	}
}

func TestTitleStreamParser_ExtractsMultilineMemoryBlock(t *testing.T) {
	parser := newTitleStreamParser(true)

	body := parser.Feed("<memory>{\n\"turn_summary\":\"第一段总结\",\n\"topic_hint\":\"测试\"\n}</memory>\n正文内容")
	if body != "正文内容" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := parser.Memory(); got != "{\n\"turn_summary\":\"第一段总结\",\n\"topic_hint\":\"测试\"\n}" {
		t.Fatalf("unexpected multiline memory: %q", got)
	}
}

func TestTitleStreamParser_ExtractsMetaInMiddleAndTail(t *testing.T) {
	parser := newTitleStreamParser(true)

	body := parser.Feed("正文A<memory>{\"turn_summary\":\"中间总结\"}</memory>结尾")
	if body != "正文A结尾" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := parser.Memory(); got != "{\"turn_summary\":\"中间总结\"}" {
		t.Fatalf("unexpected memory: %q", got)
	}
}

func TestTitleStreamParser_HandlesSplitMemoryTagAcrossChunks(t *testing.T) {
	parser := newTitleStreamParser(true)

	first := parser.Feed("前缀<mem")
	if first != "前缀" {
		t.Fatalf("unexpected first chunk output: %q", first)
	}
	second := parser.Feed("ory>{\"turn_summary\":\"跨块总结\"}</memory>后缀")
	if second != "后缀" {
		t.Fatalf("unexpected second chunk output: %q", second)
	}
	if got := parser.Memory(); got != "{\"turn_summary\":\"跨块总结\"}" {
		t.Fatalf("unexpected memory: %q", got)
	}
}

func TestTitleStreamParser_FlushIncompleteTagAsPlainText(t *testing.T) {
	parser := newTitleStreamParser(true)

	if body := parser.Feed("正文<memory>"); body != "正文" {
		t.Fatalf("unexpected body before flush: %q", body)
	}
	rest := parser.Flush()
	if rest != "<memory>" {
		t.Fatalf("expected incomplete tag passthrough, got: %q", rest)
	}
}

func TestCleanProtocolMemory_NoHardTruncate(t *testing.T) {
	longText := strings.Repeat("长", 1500)
	if got := cleanProtocolMemory(longText); got != longText {
		t.Fatalf("memory should keep full content, len=%d got=%d", len([]rune(longText)), len([]rune(got)))
	}
}

func TestExtractProtocolMetaAndBody_FallbackCleanup(t *testing.T) {
	memory, body := extractProtocolMetaAndBody("前置说明\n<title>回退标题</title>\n<memory>{\"turn_summary\":\"回退总结\"}</memory>\n最终正文")
	if memory != "{\"turn_summary\":\"回退总结\"}" {
		t.Fatalf("unexpected extracted memory: %q", memory)
	}
	if body != "前置说明\n最终正文" {
		t.Fatalf("unexpected cleaned body: %q", body)
	}
}

func TestExtractProtocolMetaAndBody_RemovesEmptyTagBlocks(t *testing.T) {
	memory, body := extractProtocolMetaAndBody("A<title></title>B<memory> </memory>C")
	if memory != "" {
		t.Fatalf("expected empty memory, got: %q", memory)
	}
	if body != "ABC" {
		t.Fatalf("unexpected cleaned body: %q", body)
	}
}

func TestExtractProtocolMetaAndBody_PreservesBodyParagraphSpacing(t *testing.T) {
	memory, body := extractProtocolMetaAndBody("第一段\n\n<title>标题</title>\n\n第二段\n\n第三段")
	if memory != "" {
		t.Fatalf("unexpected memory: %q", memory)
	}
	if body != "第一段\n\n第二段\n\n第三段" {
		t.Fatalf("unexpected cleaned body: %q", body)
	}
}

func TestExtractProtocolMetaAndBody_TrimsOnlyAdjacentProtocolWhitespace(t *testing.T) {
	memory, body := extractProtocolMetaAndBody("正文A\n \t\r\n<title>标题</title>\n\t \r\n正文B")
	if memory != "" {
		t.Fatalf("unexpected memory: %q", memory)
	}
	if body != "正文A\n正文B" {
		t.Fatalf("unexpected cleaned body: %q", body)
	}
}

func TestHandleChatStream_PersistsThinkingHistory(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	model, err := repo.CreateLLMConfig(domain.LLMConfig{
		Name:     "fake",
		Provider: llmsvc.ProviderOpenAI,
		BaseURL:  "http://fake",
		APIKey:   "key",
		Model:    "fake-model",
	})
	if err != nil {
		t.Fatalf("create model failed: %v", err)
	}
	provider := &fakeThinkingProvider{}
	svc := NewChatService(repo, nil, llmsvc.NewFactory(provider), nil, nil, nil)

	result, err := svc.HandleChatStream(ctx, session.ID, "request-1", "hello", "", model.ID, nil, "high", false, AgentCallbacks{
		OnChunk: func(string) error { return nil },
	})
	if err != nil {
		t.Fatalf("HandleChatStream failed: %v", err)
	}
	if result == nil || !strings.Contains(result.Answer, "<!-- THINKING:") {
		t.Fatalf("expected stored answer to contain thinking marker, got %#v", result)
	}

	messages, _, err := repo.ListSessionMessagesPage(session.ID, 10, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("list messages failed: %v", err)
	}
	var assistantID string
	for _, message := range messages {
		if message.Role == "assistant" {
			assistantID = message.ID
			if !strings.Contains(message.Content, "<!-- THINKING:") {
				t.Fatalf("assistant content missing thinking marker: %q", message.Content)
			}
		}
	}
	if assistantID == "" {
		t.Fatal("expected assistant message")
	}
	records, err := repo.ListSessionThinkingRecordsByAssistantMessageIDs(session.ID, []string{assistantID})
	if err != nil {
		t.Fatalf("list thinking records failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 thinking record, got %d", len(records))
	}
	if records[0].Content != "reasoning" || records[0].Status != "completed" {
		t.Fatalf("unexpected thinking record: %+v", records[0])
	}
}

func TestHandleChatStream_FinishesThinkingBeforeAnswerChunk(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	model, err := repo.CreateLLMConfig(domain.LLMConfig{
		Name:     "fake",
		Provider: llmsvc.ProviderOpenAI,
		BaseURL:  "http://fake",
		APIKey:   "key",
		Model:    "fake-model",
	})
	if err != nil {
		t.Fatalf("create model failed: %v", err)
	}
	provider := &fakeThinkingProvider{}
	svc := NewChatService(repo, nil, llmsvc.NewFactory(provider), nil, nil, nil)

	var events []string
	_, err = svc.HandleChatStream(ctx, session.ID, "request-1", "hello", "", model.ID, nil, "high", false, AgentCallbacks{
		OnThinkingStart: func() error {
			events = append(events, "thinking_start")
			return nil
		},
		OnThinkingChunk: func(string) error {
			events = append(events, "thinking_chunk")
			return nil
		},
		OnThinkingDone: func() error {
			events = append(events, "thinking_done")
			return nil
		},
		OnChunk: func(string) error {
			events = append(events, "chunk")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("HandleChatStream failed: %v", err)
	}

	want := []string{"thinking_start", "thinking_chunk", "thinking_done", "chunk"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected event order: got %v want %v", events, want)
	}
}

func TestHandleChatStream_UsesDisplayContentForStoredUserMessage(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	model, err := repo.CreateLLMConfig(domain.LLMConfig{
		Name:     "fake",
		Provider: llmsvc.ProviderOpenAI,
		BaseURL:  "http://fake",
		APIKey:   "key",
		Model:    "fake-model",
	})
	if err != nil {
		t.Fatalf("create model failed: %v", err)
	}
	provider := &captureMessagesProvider{}
	svc := NewChatService(repo, nil, llmsvc.NewFactory(provider), nil, nil, nil)

	internalPrompt := "Execute the following approved plan:\n\n# Plan"
	displayContent := "Execute this plan"
	_, err = svc.HandleChatStream(ctx, session.ID, "request-1", internalPrompt, displayContent, model.ID, nil, "off", false, AgentCallbacks{
		OnChunk: func(string) error { return nil },
	})
	if err != nil {
		t.Fatalf("HandleChatStream failed: %v", err)
	}

	messages, _, err := repo.ListSessionMessagesPage(session.ID, 10, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("list messages failed: %v", err)
	}
	var storedUser string
	for _, message := range messages {
		if message.Role == "user" {
			storedUser = message.Content
			break
		}
	}
	if storedUser != displayContent {
		t.Fatalf("stored user message = %q, want %q", storedUser, displayContent)
	}
	if len(provider.messages) == 0 {
		t.Fatal("expected provider messages")
	}
	last := provider.messages[len(provider.messages)-1]
	if last.Role != "user" || !strings.Contains(last.Content, internalPrompt) {
		t.Fatalf("provider latest user message = (%q, %q), want internal prompt", last.Role, last.Content)
	}
}

func TestHandleChatStream_PlanModeSavesOnlyPlanBody(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	model, err := repo.CreateLLMConfig(domain.LLMConfig{
		Name:     "fake",
		Provider: llmsvc.ProviderOpenAI,
		BaseURL:  "http://fake",
		APIKey:   "key",
		Model:    "fake-model",
	})
	if err != nil {
		t.Fatalf("create model failed: %v", err)
	}
	planService, err := plansvc.NewPlanService()
	if err != nil {
		t.Fatalf("create plan service failed: %v", err)
	}
	provider := &fakePlanModeProvider{}
	svc := NewChatService(repo, nil, llmsvc.NewFactory(provider), nil, nil, nil)
	svc.SetPlanService(planService)

	result, err := svc.HandleChatStream(ctx, session.ID, "request-1", "make a plan", "", model.ID, nil, "high", true, AgentCallbacks{
		OnChunk:         func(string) error { return nil },
		OnThinkingStart: func() error { return nil },
		OnThinkingChunk: func(string) error { return nil },
		OnThinkingDone:  func() error { return nil },
		OnPlanStart:     func() error { return nil },
		OnPlanChunk:     func(string) error { return nil },
		OnPlanBody:      func(string) error { return nil },
	})
	if err != nil {
		t.Fatalf("HandleChatStream failed: %v", err)
	}
	if result.PlanID == "" {
		t.Fatal("expected plan to be saved")
	}

	plans, err := planService.GetPlansBySession(session.ID)
	if err != nil {
		t.Fatalf("list plans failed: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if plans[0].Content != "# Plan\n\nDo it." {
		t.Fatalf("saved plan content = %q", plans[0].Content)
	}
	for _, marker := range []string{"<!-- THINKING:", "<!-- TOOL_CALL:", "<!-- PLAN_START -->", "<!-- PLAN_END -->", "Narration before plan."} {
		if strings.Contains(plans[0].Content, marker) {
			t.Fatalf("saved plan content should not contain %q: %q", marker, plans[0].Content)
		}
	}
	if !strings.Contains(result.Answer, "<!-- THINKING:") || !strings.Contains(result.Answer, "<!-- PLAN_START -->") {
		t.Fatalf("assistant answer should retain history markers, got %q", result.Answer)
	}
}

type captureMessagesProvider struct {
	messages []llmsvc.ChatMessage
}

func (p *captureMessagesProvider) StreamChatWithTools(
	_ context.Context,
	_ llmsvc.ModelRuntimeConfig,
	messages []llmsvc.ChatMessage,
	_ []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	p.messages = append([]llmsvc.ChatMessage{}, messages...)
	if callbacks.OnChunk != nil {
		if err := callbacks.OnChunk("answer"); err != nil {
			return nil, err
		}
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

type fakeThinkingProvider struct{}

func (p *fakeThinkingProvider) StreamChatWithTools(
	_ context.Context,
	_ llmsvc.ModelRuntimeConfig,
	_ []llmsvc.ChatMessage,
	_ []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	if callbacks.OnThinkingChunk != nil {
		if err := callbacks.OnThinkingChunk("reasoning"); err != nil {
			return nil, err
		}
	}
	if callbacks.OnChunk != nil {
		if err := callbacks.OnChunk("answer"); err != nil {
			return nil, err
		}
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

type fakePlanModeProvider struct {
	call int
}

func (p *fakePlanModeProvider) StreamChatWithTools(
	_ context.Context,
	_ llmsvc.ModelRuntimeConfig,
	_ []llmsvc.ChatMessage,
	_ []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	p.call++
	switch p.call {
	case 1:
		if callbacks.OnThinkingChunk != nil {
			if err := callbacks.OnThinkingChunk("narration thought"); err != nil {
				return nil, err
			}
		}
		if callbacks.OnChunk != nil {
			if err := callbacks.OnChunk("Narration before plan."); err != nil {
				return nil, err
			}
		}
		return &llmsvc.StreamResult{
			Type: llmsvc.StreamResultToolCalls,
			ToolCalls: []llmsvc.ToolCallInfo{{
				ID:        "plan-start-call",
				Name:      constants.PlanStartTool,
				Arguments: "{}",
			}},
			AssistantMessage: llmsvc.ChatMessage{
				Role:    "assistant",
				Content: "Narration before plan.",
				ToolCalls: []llmsvc.ToolCallInfo{{
					ID:        "plan-start-call",
					Name:      constants.PlanStartTool,
					Arguments: "{}",
				}},
			},
		}, nil
	default:
		if callbacks.OnThinkingChunk != nil {
			if err := callbacks.OnThinkingChunk("plan thought"); err != nil {
				return nil, err
			}
		}
		if callbacks.OnChunk != nil {
			if err := callbacks.OnChunk("# Plan\n\nDo it."); err != nil {
				return nil, err
			}
		}
		return &llmsvc.StreamResult{
			Type: llmsvc.StreamResultToolCalls,
			ToolCalls: []llmsvc.ToolCallInfo{{
				ID:        "plan-complete-call",
				Name:      constants.PlanCompleteTool,
				Arguments: "{}",
			}},
			AssistantMessage: llmsvc.ChatMessage{
				Role:    "assistant",
				Content: "# Plan\n\nDo it.",
				ToolCalls: []llmsvc.ToolCallInfo{{
					ID:        "plan-complete-call",
					Name:      constants.PlanCompleteTool,
					Arguments: "{}",
				}},
			},
		}, nil
	}
}

type stubTitleUpdateStore struct {
	updated bool
	err     error

	mu       sync.Mutex
	calls    int
	lastID   string
	lastName string
	done     chan struct{}
}

func (s *stubTitleUpdateStore) UpdateSessionTitle(_ context.Context, id, name string) (bool, error) {
	s.mu.Lock()
	s.calls++
	s.lastID = id
	s.lastName = name
	done := s.done
	s.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		default:
			close(done)
		}
	}
	return s.updated, s.err
}

func (s *stubTitleUpdateStore) snapshot() (calls int, id string, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.lastID, s.lastName
}

func BenchmarkTitleStreamParser_Feed(b *testing.B) {
	payload := strings.Repeat("正文内容。", 512) + "<memory>{\"turn_summary\":\"这是记忆\"}</memory>"
	for i := 0; i < b.N; i++ {
		parser := newTitleStreamParser(true)
		parser.Feed(payload)
		parser.Flush()
	}
}

func TestReadAttachmentExcerpt_TruncatesLargeText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "long.txt")
	raw := strings.Repeat("x", maxAttachmentExcerptBytes*2)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	text, ok := readAttachmentExcerpt(path, "text/plain", "txt")
	if !ok {
		t.Fatal("expected excerpt to be available")
	}
	if len(text) == 0 || len(text) > maxAttachmentExcerptBytes {
		t.Fatalf("unexpected excerpt length: %d", len(text))
	}
}

func TestReadAttachmentExcerpt_SkipsUnsupportedBinaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.bin")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	_, ok := readAttachmentExcerpt(path, "application/octet-stream", "bin")
	if ok {
		t.Fatal("expected excerpt disabled for unsupported binary files")
	}
}

func TestSystemPrompt_UsesStructuredMemoryProtocol(t *testing.T) {
	content := prompts.SystemPrompt()
	if strings.TrimSpace(content) == "" {
		t.Fatal("embedded system prompt is empty")
	}
	if strings.Contains(content, `{"facts":[...]}`) {
		t.Fatal(`system prompt must not instruct the model to emit {"facts":[...]}`)
	}
	required := []string{
		`{"name":"...","description":"...","type":"...","content":"..."}`,
		`<memory>`,
		"`type` must be one of:",
		"`user`",
		"`project`",
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("system prompt missing memory protocol token %q", token)
		}
	}
}

func TestIsInitialSessionName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "", want: true},
		{name: "   ", want: true},
		{name: "New Chat", want: true},
		{name: " New Chat ", want: true},
		{name: "New Session", want: true},
		{name: "新会话", want: true},
		{name: "未命名会话", want: true},
		{name: "我的会话", want: false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.name), func(t *testing.T) {
			if got := isInitialSessionName(tt.name); got != tt.want {
				t.Fatalf("isInitialSessionName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestMaybeGenerateTitleAsync_TriggersForInitialSessionName(t *testing.T) {
	store := &stubTitleUpdateStore{updated: true, done: make(chan struct{})}
	gen := newTitleGenerator(llmsvc.NewFactory(&fakeTitleProvider{title: `{"title":"自动标题"}`}), store)
	svc := &ChatService{titleGen: gen}

	resultCh := make(chan string, 1)
	svc.maybeGenerateTitleAsync(&domain.Session{ID: "sid-1", Name: " New Chat "}, llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI}, "用户消息", "助手回复", func(sessionID, title string) {
		resultCh <- sessionID + ":" + title
	})

	select {
	case <-store.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for title persistence")
	}

	calls, id, name := store.snapshot()
	if calls != 1 {
		t.Fatalf("UpdateSessionTitle calls = %d, want 1", calls)
	}
	if id != "sid-1" {
		t.Fatalf("persisted session id = %q, want sid-1", id)
	}
	if name != "自动标题" {
		t.Fatalf("persisted title = %q, want 自动标题", name)
	}

	select {
	case got := <-resultCh:
		if got != "sid-1:自动标题" {
			t.Fatalf("unexpected callback payload: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for title callback")
	}
}

func TestMaybeGenerateTitleAsync_SkipsWhenPreconditionsFail(t *testing.T) {
	tests := []struct {
		name        string
		session     *domain.Session
		prepare     func(*ChatService, *stubTitleUpdateStore)
		userContent string
	}{
		{
			name:        "locked title",
			session:     &domain.Session{ID: "sid-1", Name: "New Chat", IsTitleLocked: true},
			userContent: "用户消息",
		},
		{
			name:        "generator unavailable",
			session:     &domain.Session{ID: "sid-2", Name: "New Chat"},
			userContent: "用户消息",
		},
		{
			name:        "already attempted",
			session:     &domain.Session{ID: "sid-3", Name: "New Chat"},
			userContent: "用户消息",
			prepare: func(svc *ChatService, _ *stubTitleUpdateStore) {
				svc.titleGen.markAttempted("sid-3")
			},
		},
		{
			name:        "non initial name",
			session:     &domain.Session{ID: "sid-4", Name: "我的会话"},
			userContent: "用户消息",
		},
		{
			name:        "empty user message",
			session:     &domain.Session{ID: "sid-5", Name: "New Chat"},
			userContent: "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubTitleUpdateStore{updated: true, done: make(chan struct{})}
			svc := &ChatService{titleGen: newTitleGenerator(llmsvc.NewFactory(&fakeTitleProvider{title: `{"title":"自动标题"}`}), store)}
			if tt.name == "generator unavailable" {
				svc.titleGen = nil
			}
			if tt.prepare != nil {
				tt.prepare(svc, store)
			}

			called := make(chan struct{}, 1)
			svc.maybeGenerateTitleAsync(tt.session, llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI}, tt.userContent, "助手回复", func(string, string) {
				called <- struct{}{}
			})

			select {
			case <-store.done:
				t.Fatalf("UpdateSessionTitle should not be called for case %q", tt.name)
			case <-time.After(150 * time.Millisecond):
			}
			select {
			case <-called:
				t.Fatalf("callback should not be called for case %q", tt.name)
			default:
			}

			calls, _, _ := store.snapshot()
			if calls != 0 {
				t.Fatalf("UpdateSessionTitle calls = %d, want 0", calls)
			}
		})
	}
}

func TestMaybeGenerateTitleAsync_DoesNotCallbackWhenPersistReturnsFalse(t *testing.T) {
	store := &stubTitleUpdateStore{updated: false, done: make(chan struct{})}
	svc := &ChatService{titleGen: newTitleGenerator(llmsvc.NewFactory(&fakeTitleProvider{title: `{"title":"自动标题"}`}), store)}
	called := make(chan struct{}, 1)

	svc.maybeGenerateTitleAsync(&domain.Session{ID: "sid-6", Name: "New Chat"}, llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI}, "用户消息", "助手回复", func(string, string) {
		called <- struct{}{}
	})

	select {
	case <-store.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for title persistence")
	}
	select {
	case <-called:
		t.Fatal("callback should not be called when persist returns false")
	default:
	}
}

type fakeTitleProvider struct {
	title string
	err   error
}

func (p *fakeTitleProvider) StreamChatWithTools(
	_ context.Context,
	_ llmsvc.ModelRuntimeConfig,
	_ []llmsvc.ChatMessage,
	_ []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	if callbacks.OnChunk != nil {
		if err := callbacks.OnChunk(p.title); err != nil {
			return nil, err
		}
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

func TestMaybeGenerateTitleAsync_IgnoresGenerationError(t *testing.T) {
	store := &stubTitleUpdateStore{updated: true, done: make(chan struct{})}
	svc := &ChatService{titleGen: newTitleGenerator(llmsvc.NewFactory(&fakeTitleProvider{err: fmt.Errorf("boom")}), store)}
	called := make(chan struct{}, 1)

	svc.maybeGenerateTitleAsync(&domain.Session{ID: "sid-7", Name: "New Chat"}, llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI}, "用户消息", "助手回复", func(string, string) {
		called <- struct{}{}
	})

	select {
	case <-store.done:
		t.Fatal("UpdateSessionTitle should not be called on generation error")
	case <-time.After(150 * time.Millisecond):
	}
	select {
	case <-called:
		t.Fatal("callback should not be called on generation error")
	default:
	}
}

func TestMaybeGenerateTitleAsync_IgnoresPersistError(t *testing.T) {
	store := &stubTitleUpdateStore{updated: true, err: fmt.Errorf("persist failed"), done: make(chan struct{})}
	svc := &ChatService{titleGen: newTitleGenerator(llmsvc.NewFactory(&fakeTitleProvider{title: `{"title":"自动标题"}`}), store)}
	called := make(chan struct{}, 1)

	svc.maybeGenerateTitleAsync(&domain.Session{ID: "sid-8", Name: "New Chat"}, llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI}, "用户消息", "助手回复", func(string, string) {
		called <- struct{}{}
	})

	select {
	case <-store.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for persist attempt")
	}
	select {
	case <-called:
		t.Fatal("callback should not be called on persist error")
	default:
	}
}
