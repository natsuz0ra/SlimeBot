package llm

import (
	"errors"
	"testing"
)

func TestRequestOutputAndThinkingBudget(t *testing.T) {
	c, err := ResolveRequestConfig(ModelRuntimeConfig{Provider: ProviderAnthropic, ContextSize: 32768, ThinkingLevel: "medium", MaxOutputTokens: 4096})
	if err != nil || c.MaxOutputTokens != 16385 {
		t.Fatalf("thinking/output mismatch: %+v %v", c, err)
	}
	if _, err := ResolveRequestConfig(ModelRuntimeConfig{Provider: ProviderAnthropic, ContextSize: 32768, ThinkingLevel: "max"}); err == nil {
		t.Fatal("incompatible thinking window accepted")
	}
}
func TestInputUsageDoesNotDoubleCountCachesOrIncludeOutput(t *testing.T) {
	openai := TokenUsage{InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 80, TotalTokens: 150}
	if openai.InputContextTokens(ProviderOpenAI) != 100 {
		t.Fatal("cached input/output counted twice")
	}
	anthropic := TokenUsage{InputTokens: 10, CacheReadInputTokens: 80, CacheCreationInputTokens: 20, OutputTokens: 50, TotalTokens: 160}
	if anthropic.InputContextTokens(ProviderAnthropic) != 110 {
		t.Fatal("anthropic cached input omitted")
	}
}
func TestOverflowClassificationRequiresSpecificContextSemantics(t *testing.T) {
	for _, s := range []string{"400 bad request", "401 unauthorized", "429 rate limit", "invalid tool_call_id"} {
		if IsContextWindowExceeded(ClassifyContextError(errors.New(s))) {
			t.Fatalf("misclassified %s", s)
		}
	}
	for _, s := range []string{"context_length_exceeded", "maximum context length is 4096", "prompt is too long"} {
		if !IsContextWindowExceeded(ClassifyContextError(errors.New(s))) {
			t.Fatalf("missing overflow %s", s)
		}
	}
}
