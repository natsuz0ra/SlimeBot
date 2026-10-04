package contextsvc

import (
	"fmt"
	"regexp"
	llm "slimebot/internal/services/llm"
	"strings"
	"unicode/utf8"
)

var diagnosticLine = regexp.MustCompile(`(?i)(error|fatal|panic|exception|failed|failure|exit.?code|exit.?status|错误|失败|异常|^\$ |\.(go|ts|vue|js|py|rs):[0-9]+)`)
var noise = regexp.MustCompile(`\b\d{4}-\d\d-\d\d[T ][0-9:.Z+-]+\b|0x[0-9a-fA-F]{8,}`)

func clip(s string, limit int, tail bool) string {
	if EstimateText(s) <= limit {
		return s
	}
	rs := []rune(s)
	lo, hi := 0, len(rs)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		part := rs[:mid]
		if tail {
			part = rs[len(rs)-mid:]
		}
		if EstimateText(string(part)) <= limit {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if tail {
		return string(rs[len(rs)-lo:])
	}
	return string(rs[:lo])
}

// PruneText never changes the persisted tool result. Numeric error identities are retained.
func PruneText(text string, limit int, source string) string {
	if EstimateText(text) <= limit {
		return text
	}
	if limit < 192 {
		return clip("[tool output omitted; use context_read with source "+source+"]", limit, false)
	}
	var anchors []string
	seen := map[string]bool{}
	remaining := limit * 35 / 100
	for _, line := range strings.Split(text, "\n") {
		if !diagnosticLine.MatchString(line) {
			continue
		}
		key := noise.ReplaceAllString(line, "")
		if seen[key] {
			continue
		}
		seen[key] = true
		piece := clip(line, min(160, remaining), false)
		cost := EstimateText(piece) + 1
		if cost > remaining || piece == "" {
			continue
		}
		anchors = append(anchors, piece)
		remaining -= cost
		if remaining < 16 {
			break
		}
	}
	marker := fmt.Sprintf("\n[Tool output shortened; source=%s; original_bytes=%d. Use context_read for bounded original ranges.]\n", source, len(text))
	allowance := max(0, limit-EstimateText(marker)-EstimateText(strings.Join(anchors, "\n"))-32)
	head := clip(text, allowance/2, false)
	tail := clip(text, allowance/2, true)
	if idx := strings.LastIndex(head, "\n"); idx > len(head)/2 {
		head = head[:idx]
	}
	if idx := strings.Index(tail, "\n"); idx >= 0 && idx < len(tail)/2 {
		tail = tail[idx+1:]
	}
	out := head + marker + strings.Join(anchors, "\n") + "\n[recent output]\n" + tail
	if strings.Count(out, "```")%2 != 0 {
		out += "\n```"
	}
	if !utf8.ValidString(out) || EstimateText(out) > limit || len(out) >= len(text) {
		return clip(marker+strings.Join(anchors, "\n"), limit, false)
	}
	return out
}

func pruneMessage(m llm.ChatMessage, limit int, source string) llm.ChatMessage {
	if m.Role != "tool" {
		return m
	}
	m.ContentParts = append([]llm.ChatMessageContentPart(nil), m.ContentParts...)
	if len(m.ContentParts) > 0 {
		// Text-only results may be shortened; rich block order and types are untouched.
		if len(m.ContentParts) == 1 && m.ContentParts[0].Type == llm.ChatMessageContentPartTypeText {
			m.ContentParts[0].Text = PruneText(m.ContentParts[0].Text, limit, source)
		}
	} else {
		m.Content = PruneText(m.Content, limit, source)
	}
	return m
}
