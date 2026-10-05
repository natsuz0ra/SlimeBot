package contextsvc

import (
	"encoding/json"
	"fmt"
	llm "slimebot/internal/services/llm"
	"unicode"
)

type Budget struct{ Window, Output, Safety, HardInput, Pressure, Target int }

func RequestBudget(c llm.ModelRuntimeConfig) (Budget, error) {
	c, err := llm.ResolveRequestConfig(c)
	if err != nil {
		return Budget{}, err
	}
	b := Budget{Window: c.ContextSize, Output: c.MaxOutputTokens, Safety: max(512, c.ContextSize*3/100)}
	b.HardInput = b.Window - b.Output - b.Safety
	b.Pressure = min(b.Window*75/100, b.HardInput)
	b.Target = min(b.Window*55/100, b.Pressure-max(128, b.Window*3/100))
	if b.HardInput <= 0 || b.Target <= 0 {
		return b, fmt.Errorf("上下文窗口不足以预留输出与安全余量")
	}
	return b, nil
}

// EstimateText is conservative for CJK/emoji and inexpensive enough for every step.
func EstimateText(s string) int {
	ascii, other := 0, 0
	for _, r := range s {
		if r < 128 {
			ascii++
		} else if unicode.Is(unicode.Han, r) || r > 0xffff {
			other += 2
		} else {
			other++
		}
	}
	return (ascii+2)/3 + other
}
func Estimate(messages []llm.ChatMessage, tools []llm.ToolDef) int {
	total := 3
	for _, m := range messages {
		total += 8 + EstimateText(m.Role) + EstimateText(m.ToolCallID) + EstimateText(m.ReasoningContent)
		if len(m.ContentParts) == 0 {
			total += EstimateText(m.Content)
		}
		for _, t := range m.ToolCalls {
			total += 12 + EstimateText(t.ID) + EstimateText(t.Name) + EstimateText(t.Arguments)
		}
		for _, t := range m.ThinkingBlocks {
			total += EstimateText(t.Thinking) + EstimateText(t.Signature) + EstimateText(t.RedactedData)
		}
		for _, p := range m.ContentParts {
			switch p.Type {
			case llm.ChatMessageContentPartTypeText:
				total += EstimateText(p.Text)
			case llm.ChatMessageContentPartTypeImage:
				if p.ImageDetail == "low" {
					total += 85
				} else {
					total += 8192
				}
			default:
				total += EstimateText(p.Text) + EstimateText(p.Filename) + len(p.FileDataBase64)/2 + len(p.InputAudioData)/100
			}
		}
	}
	if len(tools) > 0 {
		b, _ := json.Marshal(tools)
		total += 16 + EstimateText(string(b))
	}
	return total
}
