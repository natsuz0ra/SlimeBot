package memory

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slimebot/internal/domain"
	"strings"
	"time"
	"unicode"
)

var nonWordRuneRegex = regexp.MustCompile(`[^\p{L}\p{N}_\-]+`)

// parseMemoryDecision 兼容代码块包裹的 JSON 输出，提取记忆检索决策。
func parseMemoryDecision(raw string) (MemoryDecision, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return MemoryDecision{}, fmt.Errorf("memory decision is empty")
	}
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return MemoryDecision{}, fmt.Errorf("invalid memory decision JSON format")
	}
	jsonText := text[start : end+1]

	var decision MemoryDecision
	if err := json.Unmarshal([]byte(jsonText), &decision); err != nil {
		return MemoryDecision{}, err
	}
	return decision, nil
}

// flattenMessages 将消息列表压平成稳定文本，供摘要/检索提示构建使用。
func flattenMessages(messages []domain.Message) string {
	var b strings.Builder
	for _, item := range messages {
		role := strings.TrimSpace(item.Role)
		if role == "" {
			role = "unknown"
		}
		timeText := item.CreatedAt.Local().Format(time.RFC3339)
		if item.CreatedAt.IsZero() {
			timeText = "unknown-time"
		}
		b.WriteString("[")
		b.WriteString(timeText)
		b.WriteString("] ")
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(item.Content))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// splitByUnicodeWord 在分词器不可用时作为回退切词策略。
func splitByUnicodeWord(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' && r != '-'
	})
	return fields
}

// normalizeToken 统一 token 规范化并过滤过短噪声词。
func normalizeToken(token string) string {
	normalized := strings.ToLower(strings.TrimSpace(token))
	if normalized == "" {
		return ""
	}
	normalized = nonWordRuneRegex.ReplaceAllString(normalized, "")
	if normalized == "" {
		return ""
	}
	runeCount := len([]rune(normalized))
	if runeCount <= 1 {
		return ""
	}
	return normalized
}
