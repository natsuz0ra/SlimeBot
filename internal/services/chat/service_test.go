package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"slimebot/internal/domain"
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
	raw, err := os.ReadFile(filepath.Clean("../../../prompts/system_prompt.md"))
	if err != nil {
		t.Fatalf("read system prompt failed: %v", err)
	}
	content := string(raw)
	if strings.Contains(content, `{"facts":[...]}`) {
		t.Fatal(`system prompt must not instruct the model to emit {"facts":[...]}`)
	}
	required := []string{
		`"turn_summary":"..."`,
		`"topic_hint":"..."`,
		`"keywords":[...]`,
		`"sticky":[...]`,
		`"kind":"preference|constraint|task"`,
		`"action":"upsert|delete"`,
	}
	for _, token := range required {
		if !strings.Contains(content, token) {
			t.Fatalf("system prompt missing memory protocol token %q", token)
		}
	}
}
