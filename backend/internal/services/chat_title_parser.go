package services

import (
	"regexp"
	"strings"
)

var protocolGapRegex = regexp.MustCompile(`\n{2,}`)

type titleStreamParser struct {
	// 是否启用协议解析；关闭时全部透传。
	enabled bool
	// 最近一次成功解析出的标题。
	title string
	// 最近一次成功解析出的会话总结。
	summary string
	// 当前正在解析的协议标签：title / summary。
	activeTag string
	// 普通文本阶段下，可能跨 chunk 的开标签探测缓存。
	openBuf []rune
	// 标签内容阶段下，可能跨 chunk 的闭标签探测缓存。
	closeBuf []rune
	// 当前标签内容缓存（不包含标签本身）。
	tagContent []rune
	// 标记在移除协议标签后，下一段正文前缀空白应被裁剪。
	trimNextTextPrefix bool
}

// newTitleStreamParser 创建标题协议解析器；禁用时直接透传所有内容。
func newTitleStreamParser(enabled bool) *titleStreamParser {
	if !enabled {
		return &titleStreamParser{enabled: false}
	}
	return &titleStreamParser{enabled: true}
}

// Feed 增量接收模型流片段并返回可直接下发前端的正文部分。
func (p *titleStreamParser) Feed(chunk string) string {
	if chunk == "" {
		return ""
	}
	if !p.enabled {
		return chunk
	}
	var out strings.Builder
	for _, r := range chunk {
		if p.activeTag == "" {
			p.consumeTextRune(r, &out)
			continue
		}
		p.consumeTagRune(r)
	}
	return out.String()
}

// Flush 在流结束时冲刷缓冲中的残留内容。
func (p *titleStreamParser) Flush() string {
	if !p.enabled {
		return ""
	}

	var out strings.Builder
	if p.activeTag != "" {
		// 标签未闭合，按正文透传，避免吞字。
		out.WriteString("<")
		out.WriteString(p.activeTag)
		out.WriteString(">")
		out.WriteString(string(p.tagContent))
		out.WriteString(string(p.closeBuf))
	} else if len(p.openBuf) > 0 {
		// 开标签探测缓存残留，说明不是完整协议标签，按正文透传。
		out.WriteString(string(p.openBuf))
	}
	p.resetStreamState()
	return out.String()
}

func (p *titleStreamParser) Title() string {
	return p.title
}

func (p *titleStreamParser) Summary() string {
	return p.summary
}

// BeginAssistantTurn 在工具调用切轮时重置探测状态，避免协议跨轮污染。
func (p *titleStreamParser) BeginAssistantTurn() string {
	if !p.enabled {
		return ""
	}
	// 工具调用切轮时先冲刷残留，避免跨轮污染解析状态。
	return p.Flush()
}

func (p *titleStreamParser) consumeTextRune(r rune, out *strings.Builder) {
	if p.trimNextTextPrefix && len(p.openBuf) == 0 {
		if isProtocolSeparatorRune(r) {
			return
		}
		// 若后续直接进入下一个协议标签，继续保持裁剪标记；
		// 若进入正文，则关闭裁剪标记。
		if r != '<' {
			p.trimNextTextPrefix = false
		}
	}

	if len(p.openBuf) == 0 {
		if r == '<' {
			p.openBuf = append(p.openBuf, r)
			return
		}
		out.WriteRune(r)
		p.trimNextTextPrefix = false
		return
	}

	p.openBuf = append(p.openBuf, r)
	for len(p.openBuf) > 0 && !isOpenTagPrefix(string(p.openBuf)) {
		out.WriteRune(p.openBuf[0])
		p.trimNextTextPrefix = false
		p.openBuf = p.openBuf[1:]
	}
	if len(p.openBuf) == 0 {
		return
	}

	if tag, ok := matchOpenTag(string(p.openBuf)); ok {
		p.activeTag = tag
		p.openBuf = nil
		p.closeBuf = nil
		p.tagContent = nil
	}
}

func (p *titleStreamParser) consumeTagRune(r rune) {
	endTag := "</" + p.activeTag + ">"
	if len(p.closeBuf) == 0 {
		if r == '<' {
			p.closeBuf = append(p.closeBuf, r)
			return
		}
		p.tagContent = append(p.tagContent, r)
		return
	}

	p.closeBuf = append(p.closeBuf, r)
	for len(p.closeBuf) > 0 && !strings.HasPrefix(endTag, string(p.closeBuf)) {
		p.tagContent = append(p.tagContent, p.closeBuf[0])
		p.closeBuf = p.closeBuf[1:]
	}
	if len(p.closeBuf) == 0 {
		return
	}

	if string(p.closeBuf) == endTag {
		p.finishActiveTag()
	}
}

func (p *titleStreamParser) finishActiveTag() {
	switch p.activeTag {
	case "title":
		if cleaned := cleanProtocolTitle(string(p.tagContent)); cleaned != "" {
			p.title = cleaned
		}
	case "summary":
		if cleaned := cleanProtocolSummary(string(p.tagContent)); cleaned != "" {
			p.summary = cleaned
		}
	}
	p.activeTag = ""
	p.closeBuf = nil
	p.tagContent = nil
	p.trimNextTextPrefix = true
}

func (p *titleStreamParser) resetStreamState() {
	p.activeTag = ""
	p.openBuf = nil
	p.closeBuf = nil
	p.tagContent = nil
	p.trimNextTextPrefix = false
}

func isProtocolSeparatorRune(r rune) bool {
	switch r {
	case ' ', '\t', '\r', '\n', '\uFEFF':
		return true
	default:
		return false
	}
}

func isOpenTagPrefix(candidate string) bool {
	return strings.HasPrefix("<title>", candidate) || strings.HasPrefix("<summary>", candidate)
}

func matchOpenTag(candidate string) (string, bool) {
	switch candidate {
	case "<title>":
		return "title", true
	case "<summary>":
		return "summary", true
	default:
		return "", false
	}
}

func cleanProtocolTitle(input string) string {
	title := strings.ReplaceAll(input, "\r", "")
	title = strings.ReplaceAll(title, "\n", "")
	title = strings.Trim(title, "\"'\u201c\u201d")
	title = truncateRunes(title, 20)
	return strings.TrimSpace(title)
}

func cleanProtocolSummary(input string) string {
	summary := strings.ReplaceAll(input, "\r\n", "\n")
	summary = strings.ReplaceAll(summary, "\r", "\n")
	summary = strings.TrimSpace(summary)
	return summary
}

// extractProtocolMetaAndBody 用于兜底提取协议元信息并剔除正文中的协议行。
func extractProtocolMetaAndBody(input string) (string, string, string) {
	if strings.TrimSpace(input) == "" {
		return "", "", input
	}

	body := input
	var extractedTitle string
	var extractedSummary string
	hasTagBlock := false

	if title, cleaned, foundValue, removed := extractAndRemoveTagBlocks(body, "title", cleanProtocolTitle); removed {
		if foundValue {
			extractedTitle = title
		}
		body = cleaned
		hasTagBlock = true
	}
	if summary, cleaned, foundValue, removed := extractAndRemoveTagBlocks(body, "summary", cleanProtocolSummary); removed {
		if foundValue {
			extractedSummary = summary
		}
		body = cleaned
		hasTagBlock = true
	}

	if !hasTagBlock {
		return "", "", input
	}

	body = protocolGapRegex.ReplaceAllString(body, "\n")
	return extractedTitle, extractedSummary, strings.Trim(body, "\r\n")
}

func extractAndRemoveTagBlocks(input string, tag string, cleaner func(string) string) (string, string, bool, bool) {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"
	working := input
	latest := ""
	found := false
	removed := false
	for {
		startIdx := strings.Index(working, startTag)
		if startIdx < 0 {
			break
		}
		endRel := strings.Index(working[startIdx+len(startTag):], endTag)
		if endRel < 0 {
			break
		}
		contentStart := startIdx + len(startTag)
		contentEnd := contentStart + endRel
		if value := cleaner(working[contentStart:contentEnd]); value != "" {
			latest = value
			found = true
		}
		blockEnd := contentEnd + len(endTag)
		working = working[:startIdx] + working[blockEnd:]
		removed = true
	}
	return latest, working, found, removed
}

// truncateRunes 按 rune 截断，避免中文等多字节字符被破坏。
func truncateRunes(input string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	return string(runes[:max])
}
