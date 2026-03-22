package memory

import (
	"strings"

	"slimebot/internal/constants"
)

var defaultMemoryStopWords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "for": {}, "to": {}, "of": {}, "in": {}, "on": {}, "at": {},
	"is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "with": {}, "from": {}, "this": {}, "that": {}, "it": {},
	"你": {}, "我": {}, "他": {}, "她": {}, "它": {}, "我们": {}, "你们": {}, "他们": {}, "以及": {}, "并且": {}, "或者": {},
	"一个": {}, "一些": {}, "可以": {}, "需要": {}, "然后": {}, "就是": {}, "这里": {}, "这个": {}, "那个": {},
}

// tokenizeKeywordsImpl gse 搜索分词 + 停用词过滤 + 去重上限；无候选时回退 Unicode 切词。
// 结果数量受 MemoryKeywordMaxCount 限制。
func tokenizeKeywordsImpl(m *MemoryService, text string) []string {
	m.segOnce.Do(func() {
		// 使用嵌入词典快速初始化，避免每次分词重复加载。
		m.segmenter.LoadDict()
	})

	candidates := m.segmenter.CutSearch(strings.TrimSpace(text), true)
	if len(candidates) == 0 {
		candidates = splitByUnicodeWord(text)
	}

	result := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, token := range candidates {
		normalized := normalizeToken(token)
		if normalized == "" {
			continue
		}
		if _, skip := defaultMemoryStopWords[normalized]; skip {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
		if len(result) >= constants.MemoryKeywordMaxCount {
			break
		}
	}
	return result
}
