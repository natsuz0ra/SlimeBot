package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"
	plansvc "slimebot/internal/services/plan"
	prompts "slimebot/prompts"
)

func TestTitleStreamParser_ExtractsTitleOnFirstTurn(t *testing.T) {
	parser := newTitleStreamParser(true)

	body := parser.Feed("<title>测试标题</title>\n正文内容\n")
	if body != "正文内容\n" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := parser.Title(); got != "测试标题" {
		t.Fatalf("unexpected title: %q", got)
	}
}

func TestTitleStreamParser_ExtractsTitleAfterAssistantTurnBoundary(t *testing.T) {
	parser := newTitleStreamParser(true)

	first := parser.Feed("我先调用工具获取信息")
	if first != "我先调用工具获取信息" {
		t.Fatalf("unexpected first turn output: %q", first)
	}

	parser.BeginAssistantTurn()
	second := parser.Feed("<title>稳定标题</title>\n这是最终答案\n")
	if second != "这是最终答案\n" {
		t.Fatalf("unexpected second turn output: %q", second)
	}
	if got := parser.Title(); got != "稳定标题" {
		t.Fatalf("unexpected title after boundary reset: %q", got)
	}
}

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

	body := parser.Feed("正文A<title>中间标题</title>正文B<memory>{\"turn_summary\":\"中间总结\"}</memory>结尾")
	if body != "正文A正文B结尾" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := parser.Title(); got != "中间标题" {
		t.Fatalf("unexpected title: %q", got)
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

func TestTitleStreamParser_UsesLastValidMetaWhenRepeated(t *testing.T) {
	parser := newTitleStreamParser(true)

	body := parser.Feed("<title>旧标题</title>正文<title>新标题</title><memory>{\"turn_summary\":\"旧总结\"}</memory><memory>{\"turn_summary\":\"新总结\"}</memory>")
	if body != "正文" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := parser.Title(); got != "新标题" {
		t.Fatalf("expected latest title, got: %q", got)
	}
	if got := parser.Memory(); got != "{\"turn_summary\":\"新总结\"}" {
		t.Fatalf("expected latest memory payload, got: %q", got)
	}
}

func TestTitleStreamParser_FlushIncompleteTagAsPlainText(t *testing.T) {
	parser := newTitleStreamParser(true)

	if body := parser.Feed("正文<title>"); body != "正文" {
		t.Fatalf("unexpected body before flush: %q", body)
	}
	rest := parser.Flush()
	if rest != "<title>" {
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
	title, memory, body := extractProtocolMetaAndBody("前置说明\n<title>回退标题</title>\n<memory>{\"turn_summary\":\"回退总结\"}</memory>\n最终正文")
	if title != "回退标题" {
		t.Fatalf("unexpected extracted title: %q", title)
	}
	if memory != "{\"turn_summary\":\"回退总结\"}" {
		t.Fatalf("unexpected extracted memory: %q", memory)
	}
	if body != "前置说明\n最终正文" {
		t.Fatalf("unexpected cleaned body: %q", body)
	}
}

func TestExtractProtocolMetaAndBody_RemovesEmptyTagBlocks(t *testing.T) {
	title, memory, body := extractProtocolMetaAndBody("A<title></title>B<memory> </memory>C")
	if title != "" {
		t.Fatalf("expected empty title, got: %q", title)
	}
	if memory != "" {
		t.Fatalf("expected empty memory, got: %q", memory)
	}
	if body != "ABC" {
		t.Fatalf("unexpected cleaned body: %q", body)
	}
}

func TestExtractProtocolMetaAndBody_PreservesBodyParagraphSpacing(t *testing.T) {
	title, memory, body := extractProtocolMetaAndBody("第一段\n\n<title>标题</title>\n\n第二段\n\n第三段")
	if title != "标题" {
		t.Fatalf("unexpected title: %q", title)
	}
	if memory != "" {
		t.Fatalf("unexpected memory: %q", memory)
	}
	if body != "第一段\n\n第二段\n\n第三段" {
		t.Fatalf("unexpected cleaned body: %q", body)
	}
}

func TestExtractProtocolMetaAndBody_TrimsOnlyAdjacentProtocolWhitespace(t *testing.T) {
	title, memory, body := extractProtocolMetaAndBody("正文A\n \t\r\n<title>标题</title>\n\t \r\n正文B")
	if title != "标题" {
		t.Fatalf("unexpected title: %q", title)
	}
	if memory != "" {
		t.Fatalf("unexpected memory: %q", memory)
	}
	if body != "正文A\n正文B" {
		t.Fatalf("unexpected cleaned body: %q", body)
	}
}

func TestApplySessionTitleUpdate_OnlyMarksUpdatedWhenStoreChanges(t *testing.T) {
	svc := &ChatService{}
	ctx := context.Background()

	result := &ChatStreamResult{}
	session := &domain.Session{ID: "session-1", Name: "New Chat"}
	store := &stubTitleUpdateStore{updated: false}

	if err := svc.applySessionTitleUpdate(ctx, store, session, "自动标题", result); err != nil {
		t.Fatalf("apply title update failed: %v", err)
	}
	if result.TitleUpdated {
		t.Fatal("expected TitleUpdated to stay false when store does not update")
	}
	if result.Title != "" {
		t.Fatalf("expected empty result title, got: %q", result.Title)
	}

	store.updated = true
	if err := svc.applySessionTitleUpdate(ctx, store, session, "自动标题", result); err != nil {
		t.Fatalf("apply title update failed: %v", err)
	}
	if !result.TitleUpdated {
		t.Fatal("expected TitleUpdated to be true after successful store update")
	}
	if result.Title != "自动标题" {
		t.Fatalf("unexpected result title: %q", result.Title)
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
}

func (s *stubTitleUpdateStore) UpdateSessionTitle(_ context.Context, _, _ string) (bool, error) {
	return s.updated, s.err
}

func BenchmarkTitleStreamParser_Feed(b *testing.B) {
	payload := strings.Repeat("正文内容。", 256) + "<title>这是一个标题</title>" + strings.Repeat("更多正文。", 256) + "<memory>{\"turn_summary\":\"这是记忆\"}</memory>"
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
		`<title>`,
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
