package chat

import (
	"strings"
)

const (
	openTitleTag     = "<title>"
	closeTitleTag    = "</title>"
	openSummaryTag   = "<summary>"
	closeSummaryTag  = "</summary>"
	parserTagTitle   = "title"
	parserTagSummary = "summary"
)

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
	openBuf []byte
	// 标签内容阶段下，可能跨 chunk 的闭标签探测缓存。
	closeBuf []byte
	// 当前标签内容缓存（不包含标签本身）。
	tagContent []byte
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
	for i := 0; i < len(chunk); i++ {
		b := chunk[i]
		if p.activeTag == "" {
			p.consumeTextByte(b, &out)
			continue
		}
		p.consumeTagByte(b)
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
		out.Write(p.tagContent)
		out.Write(p.closeBuf)
	} else if len(p.openBuf) > 0 {
		// 开标签探测缓存残留，说明不是完整协议标签，按正文透传。
		out.Write(p.openBuf)
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

func (p *titleStreamParser) consumeTextByte(b byte, out *strings.Builder) {
	if p.trimNextTextPrefix && len(p.openBuf) == 0 {
		if isProtocolSeparatorByte(b) {
			return
		}
		// 若后续直接进入下一个协议标签，继续保持裁剪标记；
		// 若进入正文，则关闭裁剪标记。
		if b != '<' {
			p.trimNextTextPrefix = false
		}
	}

	if len(p.openBuf) == 0 {
		if b == '<' {
			p.openBuf = append(p.openBuf, b)
			return
		}
		out.WriteByte(b)
		p.trimNextTextPrefix = false
		return
	}

	p.openBuf = append(p.openBuf, b)
	for len(p.openBuf) > 0 && !isOpenTagPrefixBytes(p.openBuf) {
		out.WriteByte(p.openBuf[0])
		p.trimNextTextPrefix = false
		p.openBuf = p.openBuf[1:]
	}
	if len(p.openBuf) == 0 {
		return
	}

	if tag, ok := matchOpenTagBytes(p.openBuf); ok {
		p.activeTag = tag
		p.openBuf = nil
		p.closeBuf = nil
		p.tagContent = nil
	}
}

func (p *titleStreamParser) consumeTagByte(b byte) {
	endTag := parserEndTag(p.activeTag)
	if len(p.closeBuf) == 0 {
		if b == '<' {
			p.closeBuf = append(p.closeBuf, b)
			return
		}
		p.tagContent = append(p.tagContent, b)
		return
	}

	p.closeBuf = append(p.closeBuf, b)
	for len(p.closeBuf) > 0 && !hasBytePrefix(endTag, p.closeBuf) {
		p.tagContent = append(p.tagContent, p.closeBuf[0])
		p.closeBuf = p.closeBuf[1:]
	}
	if len(p.closeBuf) == 0 {
		return
	}

	if bytesEqual(endTag, p.closeBuf) {
		p.finishActiveTag()
	}
}

func (p *titleStreamParser) finishActiveTag() {
	switch p.activeTag {
	case parserTagTitle:
		if cleaned := cleanProtocolTitle(string(p.tagContent)); cleaned != "" {
			p.title = cleaned
		}
	case parserTagSummary:
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
		removeStart, removeEnd, bridge := protocolRemovalRange(working, startIdx, blockEnd)
		working = working[:removeStart] + bridge + working[removeEnd:]
		removed = true
	}
	return latest, working, found, removed
}

func isProtocolSeparatorByte(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func isOpenTagPrefixBytes(candidate []byte) bool {
	return hasBytePrefix([]byte(openTitleTag), candidate) || hasBytePrefix([]byte(openSummaryTag), candidate)
}

func matchOpenTagBytes(candidate []byte) (string, bool) {
	switch {
	case bytesEqual([]byte(openTitleTag), candidate):
		return parserTagTitle, true
	case bytesEqual([]byte(openSummaryTag), candidate):
		return parserTagSummary, true
	default:
		return "", false
	}
}

func parserEndTag(activeTag string) []byte {
	switch activeTag {
	case parserTagTitle:
		return []byte(closeTitleTag)
	case parserTagSummary:
		return []byte(closeSummaryTag)
	default:
		return nil
	}
}

func hasBytePrefix(full []byte, prefix []byte) bool {
	if len(prefix) > len(full) {
		return false
	}
	for i := range prefix {
		if full[i] != prefix[i] {
			return false
		}
	}
	return true
}

func bytesEqual(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func protocolRemovalRange(input string, startIdx int, blockEnd int) (int, int, string) {
	removeStart := startIdx
	for removeStart > 0 && isProtocolSeparatorByte(input[removeStart-1]) {
		removeStart--
	}
	removeEnd := blockEnd
	for removeEnd < len(input) && isProtocolSeparatorByte(input[removeEnd]) {
		removeEnd++
	}

	leftGap := input[removeStart:startIdx]
	rightGap := input[blockEnd:removeEnd]
	if removeStart == 0 || removeEnd == len(input) {
		return removeStart, removeEnd, ""
	}

	newlineCount := countMaxNewlineRun(leftGap)
	if rightCount := countMaxNewlineRun(rightGap); rightCount > newlineCount {
		newlineCount = rightCount
	}
	if newlineCount > 0 {
		return removeStart, removeEnd, strings.Repeat("\n", newlineCount)
	}
	if len(leftGap) > 0 || len(rightGap) > 0 {
		return removeStart, removeEnd, " "
	}
	return removeStart, removeEnd, ""
}

func countMaxNewlineRun(input string) int {
	maxRun := 0
	current := 0
	for i := 0; i < len(input); i++ {
		if input[i] == '\n' {
			current++
			if current > maxRun {
				maxRun = current
			}
			continue
		}
		if input[i] != '\r' {
			current = 0
		}
	}
	return maxRun
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
