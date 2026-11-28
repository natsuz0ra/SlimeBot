package llm

import (
	"net/url"
	"strings"
)

// ThinkingStrategy identifies the wire format used to control model thinking.
type ThinkingStrategy string

const (
	ThinkingStrategyReasoningEffort ThinkingStrategy = "reasoning_effort"
	ThinkingStrategyDeepSeek        ThinkingStrategy = "deepseek_thinking"
	ThinkingStrategyAnthropicBudget ThinkingStrategy = "anthropic_budget"
)

// ProviderCapabilities describes provider-specific request encoding decisions.
// ResolveCapabilities centralizes compatibility inference for both native and
// OpenAI-compatible endpoints.
type ProviderCapabilities struct {
	SupportsDeveloperRole   bool
	UsesMaxCompletionTokens bool
	ThinkingStrategy        ThinkingStrategy
}

// ResolveCapabilities preserves the historical URL/model-name defaults while
// exposing them as one explicit contract to provider adapters.
func ResolveCapabilities(cfg ModelRuntimeConfig) ProviderCapabilities {
	provider := NormalizeProviderName(cfg.Provider)
	if provider == ProviderAnthropic {
		return ProviderCapabilities{ThinkingStrategy: ThinkingStrategyAnthropicBudget}
	}

	capabilities := ProviderCapabilities{
		SupportsDeveloperRole: supportsDeveloperRole(cfg.BaseURL),
		ThinkingStrategy:      ThinkingStrategyReasoningEffort,
	}
	if isDeepSeekRequest(cfg, provider) {
		capabilities.ThinkingStrategy = ThinkingStrategyDeepSeek
		return capabilities
	}
	capabilities.UsesMaxCompletionTokens = capabilities.SupportsDeveloperRole && usesCompletionTokenLimit(cfg)
	return capabilities
}

func supportsDeveloperRole(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return true
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(strings.TrimSpace(parsed.Path))
	if strings.Contains(host, "dashscope.aliyuncs.com") {
		return false
	}
	if strings.Contains(host, "aliyuncs.com") && strings.Contains(path, "/compatible-mode/") {
		return false
	}
	return true
}

func usesCompletionTokenLimit(cfg ModelRuntimeConfig) bool {
	parsed, _ := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if parsed != nil && strings.EqualFold(parsed.Hostname(), "api.openai.com") {
		return true
	}
	model := strings.ToLower(strings.TrimSpace(cfg.Model))
	return strings.HasPrefix(model, "gpt-") ||
		model == "o1" || strings.HasPrefix(model, "o1-") ||
		model == "o3" || strings.HasPrefix(model, "o3-") ||
		model == "o4" || strings.HasPrefix(model, "o4-")
}

func isDeepSeekRequest(cfg ModelRuntimeConfig, normalizedProvider string) bool {
	if normalizedProvider == ProviderDeepSeek {
		return true
	}
	if normalizedProvider != ProviderOpenAI {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Model)) {
	case "deepseek-flash", "deepseek-v4-flash", "deepseek-v4-pro":
		return true
	default:
		return false
	}
}
