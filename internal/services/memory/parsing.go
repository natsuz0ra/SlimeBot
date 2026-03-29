package memory

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type stickyPayload struct {
	Kind       string  `json:"kind"`
	Key        string  `json:"key"`
	Value      string  `json:"value"`
	Summary    string  `json:"summary"`
	Confidence float64 `json:"confidence"`
	Action     string  `json:"action"`
}

func (s *stickyPayload) UnmarshalJSON(data []byte) error {
	type rawStickyPayload struct {
		Kind       string          `json:"kind"`
		Key        string          `json:"key"`
		Value      json.RawMessage `json:"value"`
		Summary    string          `json:"summary"`
		Confidence float64         `json:"confidence"`
		Action     string          `json:"action"`
	}
	var raw rawStickyPayload
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Kind = raw.Kind
	s.Key = raw.Key
	s.Value = stringifyStickyValue(raw.Value)
	s.Summary = raw.Summary
	s.Confidence = raw.Confidence
	s.Action = raw.Action
	return nil
}

type turnMemoryPayload struct {
	TurnSummary string          `json:"turn_summary"`
	TopicHint   string          `json:"topic_hint"`
	Keywords    []string        `json:"keywords"`
	Sticky      []stickyPayload `json:"sticky"`
}

func parseTurnMemoryPayload(raw string) (turnMemoryPayload, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return turnMemoryPayload{}, fmt.Errorf("empty memory payload")
	}
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return turnMemoryPayload{}, fmt.Errorf("invalid memory payload")
	}
	var payload turnMemoryPayload
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return turnMemoryPayload{}, err
	}
	payload.TurnSummary = strings.TrimSpace(payload.TurnSummary)
	payload.TopicHint = strings.TrimSpace(payload.TopicHint)
	payload.Keywords = normalizeKeywordsForPayload(payload.Keywords)
	for idx := range payload.Sticky {
		item := &payload.Sticky[idx]
		item.Kind = strings.TrimSpace(strings.ToLower(item.Kind))
		item.Key = strings.TrimSpace(strings.ToLower(item.Key))
		item.Value = strings.TrimSpace(item.Value)
		item.Summary = strings.TrimSpace(item.Summary)
		item.Action = strings.TrimSpace(strings.ToLower(item.Action))
		if item.Confidence < 0 {
			item.Confidence = 0
		}
		if item.Confidence > 1 {
			item.Confidence = 1
		}
	}
	if payload.TurnSummary == "" && payload.TopicHint == "" && len(payload.Sticky) == 0 {
		return turnMemoryPayload{}, fmt.Errorf("empty turn memory content")
	}
	return payload, nil
}

func normalizeKeywordsForPayload(keywords []string) []string {
	seen := make(map[string]struct{}, len(keywords))
	result := make([]string, 0, len(keywords))
	for _, item := range keywords {
		normalized := normalizeToken(item)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func stringifyStickyValue(raw json.RawMessage) string {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return ""
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}
	return text
}

var nonWordRuneRegex = regexp.MustCompile(`[^\p{L}\p{N}_\-]+`)

func splitByUnicodeWord(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' && r != '-'
	})
	return fields
}

func normalizeToken(token string) string {
	normalized := strings.ToLower(strings.TrimSpace(token))
	if normalized == "" {
		return ""
	}
	normalized = nonWordRuneRegex.ReplaceAllString(normalized, "")
	if normalized == "" {
		return ""
	}
	if len([]rune(normalized)) <= 1 {
		return ""
	}
	return normalized
}

func deriveTopicKey(topicHint string, keywords []string) string {
	if normalized := strings.TrimSpace(topicHint); normalized != "" {
		return normalized
	}
	items := normalizeKeywordsForPayload(keywords)
	if len(items) > 0 {
		return strings.Join(items, "-")
	}
	return ""
}

func fallbackTopicKey(summary string, keywords []string) string {
	if key := deriveTopicKey("", keywords); key != "" {
		return key
	}
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(strings.TrimSpace(summary)))
	return fmt.Sprintf("topic-%x", hasher.Sum64())
}

func mergeKeywords(existingJSON string, incoming []string) []string {
	items := append([]string{}, decodeKeywordsJSON(existingJSON)...)
	items = append(items, incoming...)
	return normalizeKeywordsForPayload(items)
}

func decodeKeywordsJSON(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}
	}
	return normalizeKeywordsForPayload(items)
}

func mergeSummary(existing string, incoming string) string {
	existing = strings.TrimSpace(existing)
	incoming = strings.TrimSpace(incoming)
	switch {
	case existing == "":
		return incoming
	case incoming == "":
		return existing
	case strings.Contains(existing, incoming):
		return existing
	default:
		return existing + "\n" + incoming
	}
}

func safeTimeGap(now time.Time, then time.Time) time.Duration {
	if now.IsZero() || then.IsZero() || now.Before(then) {
		return 0
	}
	return now.Sub(then)
}
