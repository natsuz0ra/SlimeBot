package services

import "testing"

func TestTitleStreamParser_ExtractsTitleOnFirstTurn(t *testing.T) {
	parser := newTitleStreamParser(true, titleProbeRuneLimit)

	body := parser.Feed("[TITLE]测试标题\n正文内容\n")
	if body != "正文内容\n" {
		t.Fatalf("unexpected body: %q", body)
	}
	if got := parser.Title(); got != "测试标题" {
		t.Fatalf("unexpected title: %q", got)
	}
}

func TestTitleStreamParser_ExtractsTitleAfterAssistantTurnBoundary(t *testing.T) {
	parser := newTitleStreamParser(true, titleProbeRuneLimit)

	first := parser.Feed("我先调用工具获取信息")
	if first != "我先调用工具获取信息" {
		t.Fatalf("unexpected first turn output: %q", first)
	}

	// 模拟一次 tool_call 结束后，新一轮 assistant 最终回答开始。
	parser.BeginAssistantTurn()
	second := parser.Feed("[TITLE]稳定标题\n这是最终答案\n")
	if second != "这是最终答案\n" {
		t.Fatalf("unexpected second turn output: %q", second)
	}
	if got := parser.Title(); got != "稳定标题" {
		t.Fatalf("unexpected title after boundary reset: %q", got)
	}
}

func TestExtractProtocolTitleAndBody_FallbackCleanup(t *testing.T) {
	title, body := extractProtocolTitleAndBody("前置说明\n[TITLE]回退标题\n最终正文")
	if title != "回退标题" {
		t.Fatalf("unexpected extracted title: %q", title)
	}
	if body != "前置说明\n最终正文" {
		t.Fatalf("unexpected cleaned body: %q", body)
	}
}
