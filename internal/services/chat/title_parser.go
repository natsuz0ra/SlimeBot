package chat

import (
	"strings"
)

const (
	openMemoryTag   = "<memory>"
	closeMemoryTag  = "</memory>"
	parserTagMemory = "memory"
)

type titleStreamParser struct {
	// When false, all chunks pass through unchanged.
	enabled bool
	// Last successfully parsed memory JSON payload.
	memory string
	// Active protocol tag being parsed: memory.
	activeTag string
	// Buffered bytes while detecting an opening tag across chunks.
	openBuf []byte
	// Buffered bytes while detecting a closing tag across chunks.
	closeBuf []byte
	// Accumulated inner tag content (excluding delimiters).
	tagContent []byte
	// After removing protocol tags, trim leading whitespace on the next text segment once.
	trimNextTextPrefix bool
}

// newTitleStreamParser creates the title/memory protocol parser; disabled mode passes through.
func newTitleStreamParser(enabled bool) *titleStreamParser {
	if !enabled {
		return &titleStreamParser{enabled: false}
	}
	return &titleStreamParser{enabled: true}
}

// Feed consumes one streamed chunk and returns visible body text for the client.
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

// Flush flushes buffered partial tags at end of stream.
func (p *titleStreamParser) Flush() string {
	if !p.enabled {
		return ""
	}

	var out strings.Builder
	if p.activeTag == parserTagMemory && len(p.tagContent) > 0 {
		// Unclosed memory tag: try to extract JSON from buffered content
		// (handles wrong or missing closing tags like </Memory>, </title>, etc.)
		content := string(p.tagContent)
		if len(p.closeBuf) > 0 {
			content += string(p.closeBuf)
		}
		if jsonStr, _ := extractJSONObject(content); jsonStr != "" {
			if cleaned := cleanProtocolMemory(jsonStr); cleaned != "" {
				p.memory = cleaned
			}
		} else {
			out.WriteString("<memory>")
			out.Write(p.tagContent)
			out.Write(p.closeBuf)
		}
	} else if p.activeTag != "" {
		out.WriteString("<")
		out.WriteString(p.activeTag)
		out.WriteString(">")
		out.Write(p.tagContent)
		out.Write(p.closeBuf)
	} else if len(p.openBuf) > 0 {
		out.Write(p.openBuf)
	}
	p.resetStreamState()
	return out.String()
}

func (p *titleStreamParser) Memory() string {
	return p.memory
}

// BeginAssistantTurn resets parser state when switching assistant turns after tools.
func (p *titleStreamParser) BeginAssistantTurn() string {
	if !p.enabled {
		return ""
	}
	// Flush before a new assistant turn to avoid cross-turn tag leakage.
	return p.Flush()
}

func (p *titleStreamParser) consumeTextByte(b byte, out *strings.Builder) {
	if p.trimNextTextPrefix && len(p.openBuf) == 0 {
		if isProtocolSeparatorByte(b) {
			return
		}
		// Keep trim flag if another tag follows immediately; clear once real text starts.
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
	if p.activeTag == parserTagMemory {
		if cleaned := cleanProtocolMemory(string(p.tagContent)); cleaned != "" {
			p.memory = cleaned
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

func cleanProtocolMemory(input string) string {
	memory := strings.ReplaceAll(input, "\r\n", "\n")
	memory = strings.ReplaceAll(memory, "\r", "\n")
	memory = strings.TrimSpace(memory)
	// Strip trailing non-JSON content (e.g. wrong closing tags like </title>).
	if idx := strings.LastIndex(memory, "}"); idx >= 0 && idx < len(memory)-1 {
		trailing := strings.TrimSpace(memory[idx+1:])
		if trailing != "" {
			memory = memory[:idx+1]
		}
	}
	return memory
}

// extractProtocolMetaAndBody is a fallback that strips protocol tags from the final body.
// Title tags are stripped from the body as a migration safety net (prevents leaked tags in display)
// but the title value is no longer extracted — it is discarded.
func extractProtocolMetaAndBody(input string) (string, string) {
	if strings.TrimSpace(input) == "" {
		return "", input
	}

	body := input
	var extractedMemory string
	hasTagBlock := false

	// Strip any stray <title> tags from body (migration safety net).
	if _, cleaned, _, removed := extractAndRemoveTagBlocks(body, "title", strings.TrimSpace); removed {
		body = cleaned
		hasTagBlock = true
	}
	if memory, cleaned, foundValue, removed := extractAndRemoveTagBlocks(body, "memory", cleanProtocolMemory); removed {
		if foundValue {
			extractedMemory = memory
		}
		body = cleaned
		hasTagBlock = true
	}

	if !hasTagBlock {
		return "", input
	}

	return extractedMemory, strings.Trim(body, "\r\n")
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
		contentAfterOpen := working[startIdx+len(startTag):]
		endRel := strings.Index(contentAfterOpen, endTag)

		if endRel >= 0 {
			// Normal case: closing tag found.
			contentEnd := startIdx + len(startTag) + endRel
			if value := cleaner(working[startIdx+len(startTag) : contentEnd]); value != "" {
				latest = value
				found = true
			}
			blockEnd := contentEnd + len(endTag)
			removeStart, removeEnd, bridge := protocolRemovalRange(working, startIdx, blockEnd)
			working = working[:removeStart] + bridge + working[removeEnd:]
			removed = true
		} else if tag == "memory" {
			// Fallback: closing tag missing or wrong — extract JSON directly.
			jsonStr, jsonEnd := extractJSONObject(contentAfterOpen)
			if jsonStr == "" || jsonEnd < 0 {
				break
			}
			if value := cleaner(jsonStr); value != "" {
				latest = value
				found = true
			}
			blockEnd := startIdx + len(startTag) + jsonEnd
			removeStart, removeEnd, bridge := protocolRemovalRange(working, startIdx, blockEnd)
			working = working[:removeStart] + bridge + working[removeEnd:]
			removed = true
		} else {
			break
		}
	}
	return latest, working, found, removed
}

// extractJSONObject finds the first complete JSON object in input.
// Returns the JSON string and the byte offset after the closing '}'.
// Returns ("", -1) if no valid JSON object is found.
func extractJSONObject(input string) (string, int) {
	start := strings.Index(input, "{")
	if start < 0 {
		return "", -1
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(input); i++ {
		c := input[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' && inString {
			escape = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return input[start : i+1], i + 1
			}
		}
	}
	return "", -1
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
	return hasBytePrefix([]byte(openMemoryTag), candidate)
}

func matchOpenTagBytes(candidate []byte) (string, bool) {
	if bytesEqual([]byte(openMemoryTag), candidate) {
		return parserTagMemory, true
	}
	return "", false
}

func parserEndTag(activeTag string) []byte {
	if activeTag == parserTagMemory {
		return []byte(closeMemoryTag)
	}
	return nil
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

// truncateRunes truncates by rune count to avoid splitting multibyte characters.
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
