package llm

import "testing"

func TestResolveCapabilitiesContracts(t *testing.T) {
	tests := []struct {
		name             string
		cfg              ModelRuntimeConfig
		developerRole    bool
		completionTokens bool
		thinking         ThinkingStrategy
	}{
		{
			name:             "official OpenAI retains completion token budget",
			cfg:              ModelRuntimeConfig{Provider: ProviderOpenAI, BaseURL: "https://api.openai.com/v1", Model: "deployment-alias"},
			developerRole:    true,
			completionTokens: true,
			thinking:         ThinkingStrategyReasoningEffort,
		},
		{
			name:             "DeepSeek protocol uses explicit thinking extension",
			cfg:              ModelRuntimeConfig{Provider: ProviderDeepSeek, BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat"},
			developerRole:    true,
			completionTokens: false,
			thinking:         ThinkingStrategyDeepSeek,
		},
		{
			name:             "DeepSeek model on OpenAI compatible endpoint",
			cfg:              ModelRuntimeConfig{Provider: ProviderOpenAI, BaseURL: "https://example.com/v1", Model: "deepseek-v4-pro"},
			developerRole:    true,
			completionTokens: false,
			thinking:         ThinkingStrategyDeepSeek,
		},
		{
			name:             "Anthropic uses budgeted thinking",
			cfg:              ModelRuntimeConfig{Provider: ProviderAnthropic, BaseURL: "https://api.anthropic.com", Model: "claude"},
			developerRole:    false,
			completionTokens: false,
			thinking:         ThinkingStrategyAnthropicBudget,
		},
		{
			name:             "custom compatible endpoint retains legacy defaults",
			cfg:              ModelRuntimeConfig{Provider: ProviderOpenAI, BaseURL: "https://example.com/v1", Model: "custom-model"},
			developerRole:    true,
			completionTokens: false,
			thinking:         ThinkingStrategyReasoningEffort,
		},
		{
			name:             "custom compatible reasoning model",
			cfg:              ModelRuntimeConfig{Provider: ProviderOpenAI, BaseURL: "https://example.com/v1", Model: "gpt-5-mini"},
			developerRole:    true,
			completionTokens: true,
			thinking:         ThinkingStrategyReasoningEffort,
		},
		{
			name:             "Alibaba compatible mode disables developer role and completion budget",
			cfg:              ModelRuntimeConfig{Provider: ProviderOpenAI, BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "gpt-5-alias"},
			developerRole:    false,
			completionTokens: false,
			thinking:         ThinkingStrategyReasoningEffort,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveCapabilities(tt.cfg)
			if got.SupportsDeveloperRole != tt.developerRole || got.UsesMaxCompletionTokens != tt.completionTokens || got.ThinkingStrategy != tt.thinking {
				t.Fatalf("ResolveCapabilities() = %+v, want developer=%v completion=%v thinking=%q", got, tt.developerRole, tt.completionTokens, tt.thinking)
			}
		})
	}
}
