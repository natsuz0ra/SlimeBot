package services

import "strings"

type titleStreamParser struct {
	// 是否启用协议解析；关闭时全部透传。
	enabled bool
	// 最近一次成功解析出的标题。
	title string
	// 探测模式下的行缓冲，用于按“行”识别 [TITLE] 协议。
	lineBuf strings.Builder
	// 是否处于探测模式：true=先入缓冲识别协议，false=正文直通。
	probing bool
}

// newTitleStreamParser 创建标题协议解析器；禁用时直接透传所有内容。
func newTitleStreamParser(enabled bool, probeRuneLimit int) *titleStreamParser {
	_ = probeRuneLimit
	if !enabled {
		return &titleStreamParser{enabled: false}
	}
	return &titleStreamParser{enabled: true, probing: true}
}

// Feed 增量接收模型流片段并返回可直接下发前端的正文部分。
func (p *titleStreamParser) Feed(chunk string) string {
	if chunk == "" {
		return ""
	}
	if !p.enabled {
		return chunk
	}
	return p.process(chunk, false)
}

// Flush 在流结束时冲刷缓冲中的残留内容。
func (p *titleStreamParser) Flush() string {
	if !p.enabled {
		return ""
	}
	return p.process("", true)
}

func (p *titleStreamParser) process(chunk string, flush bool) string {
	var out strings.Builder

	// 将确认属于正文的内容透传到输出。
	writePassthrough := func(content string) {
		if content == "" {
			return
		}
		out.WriteString(content)
	}

	// 刷新当前行缓冲：可在遇到换行时自然刷新，也可强制刷新（流结束/判定非协议时）。
	flushLineBuffer := func(force bool) {
		current := p.lineBuf.String()
		if current == "" {
			return
		}
		if !force && !strings.Contains(current, "\n") {
			return
		}
		line := strings.TrimSuffix(current, "\n")
		line = strings.TrimSuffix(line, "\r")
		if title, ok := parseProtocolTitle(line); ok {
			// 协议行仅用于更新标题，不进入正文输出。
			p.title = title
			p.lineBuf.Reset()
			// 处理完一行后继续处于探测模式，便于识别下一行协议。
			p.probing = true
			return
		}
		writePassthrough(current)
		p.lineBuf.Reset()
		// 仅当以换行结束时，下一字符才视作新行开头并重新探测。
		p.probing = strings.HasSuffix(current, "\n")
	}

	for i := 0; i < len(chunk); i++ {
		ch := chunk[i]
		if p.probing {
			// 探测模式：先累积到行缓冲，再判断是否为 [TITLE] 协议。
			p.lineBuf.WriteByte(ch)
			trimmedLeft := strings.TrimLeft(p.lineBuf.String(), " \t\r\n\uFEFF")
			titleTag := "[TITLE]"
			// 前缀已足够但不匹配时，立即强制刷新为正文，避免无谓等待整行。
			if len([]rune(trimmedLeft)) >= len([]rune(titleTag)) && !strings.HasPrefix(trimmedLeft, titleTag) {
				flushLineBuffer(true)
			} else {
				flushLineBuffer(false)
			}
			continue
		}

		// 直通模式：正文原样输出，换行后回到探测模式。
		out.WriteByte(ch)
		if ch == '\n' {
			p.probing = true
		}
	}

	if flush {
		// 流结束时强制处理残留缓冲，避免最后半行被遗漏。
		flushLineBuffer(true)
	}

	return out.String()
}

func (p *titleStreamParser) Title() string {
	return p.title
}

// BeginAssistantTurn 在工具调用切轮时重置探测状态，避免标题协议跨轮污染。
func (p *titleStreamParser) BeginAssistantTurn() string {
	if !p.enabled {
		return ""
	}
	// 工具调用切轮时先冲刷残留，再回到“新一行起点探测”状态。
	passthrough := p.process("", true)
	p.probing = true
	return passthrough
}

// parseProtocolTitle 识别单行 [TITLE] 协议并执行标题清洗。
func parseProtocolTitle(line string) (string, bool) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(line, "\uFEFF", ""))
	if !strings.HasPrefix(trimmed, "[TITLE]") {
		return "", false
	}

	title := strings.TrimSpace(strings.TrimPrefix(trimmed, "[TITLE]"))
	title = strings.ReplaceAll(title, "\r", "")
	title = strings.ReplaceAll(title, "\n", "")
	title = strings.Trim(title, "\"'\u201c\u201d")
	title = truncateRunes(title, 20)
	if title == "" {
		return "", false
	}
	return title, true
}

// extractProtocolTitleAndBody 用于兜底提取标题并剔除正文中的协议行。
func extractProtocolTitleAndBody(input string) (string, string) {
	if strings.TrimSpace(input) == "" {
		return "", input
	}

	// 按行扫描协议行，支持 [TITLE] 出现在首行/中间/末行。
	segments := strings.SplitAfter(input, "\n")
	if len(segments) == 0 {
		return "", input
	}

	var extractedTitle string
	var hasTitle bool
	bodySegments := make([]string, 0, len(segments))

	for _, seg := range segments {
		line := strings.TrimSuffix(seg, "\n")
		line = strings.TrimSuffix(line, "\r")
		if title, ok := parseProtocolTitle(line); ok {
			extractedTitle = title
			hasTitle = true
			continue
		}
		bodySegments = append(bodySegments, seg)
	}

	if !hasTitle {
		return "", input
	}

	return extractedTitle, strings.Join(bodySegments, "")
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
