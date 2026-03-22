package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	// 模拟一次 tool_call 结束后，新一轮 assistant 最终回答开始。
	parser.BeginAssistantTurn()
	second := parser.Feed("<title>稳定标题</title>\n这是最终答案\n")
	if second != "这是最终答案\n" {
		t.Fatalf("unexpected second turn output: %q", second)
	}
	if got := parser.Title(); got != "稳定标题" {
		t.Fatalf("unexpected title after boundary reset: %q", got)
	}
}

func TestTitleStreamParser_ExtractsSummaryAndFiltersFromBody(t *testing.T) {
	parser := newTitleStreamParser(true)

	body := parser.Feed("<summary>用户偏好中文回复，继续保留简洁风格</summary>\n正文第一段\n")
	if body != "正文第一段\n" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := parser.Summary(); got != "用户偏好中文回复，继续保留简洁风格" {
		t.Fatalf("unexpected summary: %q", got)
	}
}

func TestTitleStreamParser_ExtractsMultilineSummaryBlock(t *testing.T) {
	parser := newTitleStreamParser(true)

	body := parser.Feed("<summary>第一段总结\n第二段总结</summary>\n正文内容")
	if body != "正文内容" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := parser.Summary(); got != "第一段总结\n第二段总结" {
		t.Fatalf("unexpected multiline summary: %q", got)
	}
}

func TestTitleStreamParser_ExtractsMetaInMiddleAndTail(t *testing.T) {
	parser := newTitleStreamParser(true)

	body := parser.Feed("正文A<title>中间标题</title>正文B<summary>中间总结</summary>结尾")
	if body != "正文A正文B结尾" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := parser.Title(); got != "中间标题" {
		t.Fatalf("unexpected title: %q", got)
	}
	if got := parser.Summary(); got != "中间总结" {
		t.Fatalf("unexpected summary: %q", got)
	}
}

func TestTitleStreamParser_HandlesSplitSummaryTagAcrossChunks(t *testing.T) {
	parser := newTitleStreamParser(true)

	first := parser.Feed("前缀<sum")
	if first != "前缀" {
		t.Fatalf("unexpected first chunk output: %q", first)
	}
	second := parser.Feed("mary>跨块总结</summary>后缀")
	if second != "后缀" {
		t.Fatalf("unexpected second chunk output: %q", second)
	}
	if got := parser.Summary(); got != "跨块总结" {
		t.Fatalf("unexpected summary: %q", got)
	}
}

func TestTitleStreamParser_UsesLastValidMetaWhenRepeated(t *testing.T) {
	parser := newTitleStreamParser(true)

	body := parser.Feed("<title>旧标题</title>正文<title>新标题</title><summary>旧总结</summary><summary>新总结</summary>")
	if body != "正文" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := parser.Title(); got != "新标题" {
		t.Fatalf("expected latest title, got: %q", got)
	}
	if got := parser.Summary(); got != "新总结" {
		t.Fatalf("expected latest summary, got: %q", got)
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

func TestCleanProtocolSummary_NoHardTruncate(t *testing.T) {
	longText := strings.Repeat("长", 1500)
	if got := cleanProtocolSummary(longText); got != longText {
		t.Fatalf("summary should keep full content, len=%d got=%d", len([]rune(longText)), len([]rune(got)))
	}
}

func TestExtractProtocolMetaAndBody_FallbackCleanup(t *testing.T) {
	title, summary, body := extractProtocolMetaAndBody("前置说明\n<title>回退标题</title>\n<summary>回退总结</summary>\n最终正文")
	if title != "回退标题" {
		t.Fatalf("unexpected extracted title: %q", title)
	}
	if summary != "回退总结" {
		t.Fatalf("unexpected extracted summary: %q", summary)
	}
	if body != "前置说明\n最终正文" {
		t.Fatalf("unexpected cleaned body: %q", body)
	}
}

func TestExtractProtocolMetaAndBody_RemovesEmptyTagBlocks(t *testing.T) {
	title, summary, body := extractProtocolMetaAndBody("A<title></title>B<summary> </summary>C")
	if title != "" {
		t.Fatalf("expected empty title, got: %q", title)
	}
	if summary != "" {
		t.Fatalf("expected empty summary, got: %q", summary)
	}
	if body != "ABC" {
		t.Fatalf("unexpected cleaned body: %q", body)
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
