package prompts

import (
	"strings"
	"testing"
)

func TestSystemPrompt_InstructsVisibleReasoningToFollowUserLanguage(t *testing.T) {
	prompt := SystemPrompt()
	for _, want := range []string{
		"visible thinking/reasoning content",
		"reasoning_content",
		"do not default to English",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q", want)
		}
	}
}

func TestSystemPrompt_EncouragesBoundedSubagentDelegation(t *testing.T) {
	prompt := SystemPrompt()
	for _, want := range []string{
		"independent, bounded",
		"Prefer completing small or direct tasks yourself",
		"write the sub-agent `task` and `context` in the user's language",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q", want)
		}
	}
	if strings.Contains(prompt, "tool-heavy work") {
		t.Fatal("system prompt should not encourage tool-heavy subagent delegation")
	}
}

func TestSystemPrompt_PrefersFileReadMultiRangeSingleCall(t *testing.T) {
	prompt := SystemPrompt()
	for _, want := range []string{
		"multiple non-contiguous lines/ranges",
		"prefer one call with `requests[].ranges[]`",
		"Keep `offset/limit` for simple single-range reads",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q", want)
		}
	}
}
