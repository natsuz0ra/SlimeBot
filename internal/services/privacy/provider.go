package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"slimebot/internal/services/llm"
)

type provider struct {
	next    llm.Provider
	service *Service
}

func (s *Service) Wrap(next llm.Provider) llm.Provider { return &provider{next, s} }

func (p *provider) StreamChatWithTools(ctx context.Context, cfg llm.ModelRuntimeConfig, messages []llm.ChatMessage, defs []llm.ToolDef, callbacks llm.StreamCallbacks) (result *llm.StreamResult, err error) {
	c, err := p.service.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	mask := c.Enabled
	for _, id := range c.BypassProviderIDs {
		if id == cfg.ProviderID {
			mask = false
			break
		}
	}
	t, err := p.service.beginRequest(ctx, c, false, !mask)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return p.next.StreamChatWithTools(ctx, cfg, messages, defs, callbacks)
	}
	defer func() {
		if recover() != nil {
			result = nil
			err = errors.New("privacy processing failed; response stopped")
		}
		t.finish(err != nil)
	}()
	if mask {
		scanCtx, cancel := context.WithCancel(ctx)
		if c.ModelEnabled {
			scanCtx, cancel = context.WithTimeout(ctx, time.Duration(c.ModelTimeoutSeconds)*time.Second)
		}
		defer cancel()
		// Discover values across the entire request first. A credential learned in
		// a later tool result must also be hidden in an earlier unlabelled sentence.
		if _, _, err = t.prepare(scanCtx, messages, defs); err != nil {
			return nil, err
		}
		if t.record.Blocked {
			if callbacks.OnPrivacy != nil {
				if err = callbacks.OnPrivacy(llm.PrivacySummary{Masked: t.record.Masked, Warned: t.record.Warned, Blocked: true}); err != nil {
					return nil, err
				}
			}
			return nil, ErrBlocked
		}
		t.hits = nil
		t.record.Masked = 0
		t.record.Warned = 0
		t.record.Kinds = map[string]int{}
		t.processed = 0
		messages, defs, err = t.prepare(scanCtx, messages, defs)
		if err != nil {
			return nil, err
		}
		if t.record.Blocked {
			return nil, ErrBlocked
		}
		cancel()
	}
	if callbacks.OnPrivacy != nil && !t.record.Blocked {
		if err = callbacks.OnPrivacy(llm.PrivacySummary{Masked: t.record.Masked, Warned: t.record.Warned}); err != nil {
			return nil, err
		}
	}
	if err = t.commitMapping(); err != nil {
		return nil, err
	}
	text := restoreStream{tx: t, emit: callbacks.OnChunk}
	thinking := restoreStream{tx: t, emit: callbacks.OnThinkingChunk}
	wrapped := llm.StreamCallbacks{}
	if callbacks.OnChunk != nil {
		wrapped.OnChunk = func(chunk string) error {
			if chunk != "" {
				if err := thinking.Finish(); err != nil {
					return err
				}
			}
			return text.Write(chunk)
		}
	}
	if callbacks.OnThinkingChunk != nil {
		wrapped.OnThinkingChunk = thinking.Write
	}
	t.scanDuration = time.Since(t.record.Time)
	t.scanComplete = true
	result, err = p.next.StreamChatWithTools(ctx, cfg, messages, defs, wrapped)
	if err != nil {
		return nil, err
	}
	if err = text.Finish(); err != nil {
		return nil, err
	}
	if err = thinking.Finish(); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("model returned an empty response")
	}
	out := *result
	out.AssistantMessage = result.AssistantMessage
	restoredBefore := t.record.Restored
	if out.AssistantMessage.Content, err = t.restore(result.AssistantMessage.Content); err != nil {
		return nil, err
	}
	if callbacks.OnChunk != nil {
		t.record.Restored = restoredBefore
	}
	restoredBefore = t.record.Restored
	if out.AssistantMessage.ReasoningContent, err = t.restore(result.AssistantMessage.ReasoningContent); err != nil {
		return nil, err
	}
	if callbacks.OnThinkingChunk != nil {
		t.record.Restored = restoredBefore
	}
	// Signed provider blocks remain in their original wire form for the next
	// turn. Only the display callback above is restored.
	out.ToolCalls = append([]llm.ToolCallInfo(nil), result.ToolCalls...)
	for i := range out.ToolCalls {
		if out.ToolCalls[i].Arguments, err = t.restoreJSON(out.ToolCalls[i].Arguments); err != nil {
			return nil, err
		}
	}
	out.AssistantMessage.ToolCalls = append([]llm.ToolCallInfo(nil), result.AssistantMessage.ToolCalls...)
	for i := range out.AssistantMessage.ToolCalls {
		matched := false
		for _, call := range out.ToolCalls {
			if call.ID != "" && call.ID == out.AssistantMessage.ToolCalls[i].ID && call.Name == out.AssistantMessage.ToolCalls[i].Name {
				out.AssistantMessage.ToolCalls[i].Arguments = call.Arguments
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if out.AssistantMessage.ToolCalls[i].Arguments, err = t.restoreJSON(out.AssistantMessage.ToolCalls[i].Arguments); err != nil {
			return nil, err
		}
	}
	return &out, nil
}

func (t *transaction) prepare(ctx context.Context, messages []llm.ChatMessage, defs []llm.ToolDef) ([]llm.ChatMessage, []llm.ToolDef, error) {
	out := append([]llm.ChatMessage(nil), messages...)
	for i, m := range messages {
		var err error
		if out[i].Content, err = t.maskText(ctx, m.Content, ""); err != nil {
			return nil, nil, err
		}
		if out[i].ReasoningContent, err = t.maskText(ctx, m.ReasoningContent, ""); err != nil {
			return nil, nil, err
		}
		out[i].ContentParts = append([]llm.ChatMessageContentPart(nil), m.ContentParts...)
		for j, part := range m.ContentParts {
			if out[i].ContentParts[j].Filename, err = t.maskText(ctx, part.Filename, ""); err != nil {
				return nil, nil, err
			}
			if part.Type == llm.ChatMessageContentPartTypeText {
				if out[i].ContentParts[j].Text, err = t.maskText(ctx, part.Text, ""); err != nil {
					return nil, nil, err
				}
			}
		}
		out[i].ToolCalls = append([]llm.ToolCallInfo(nil), m.ToolCalls...)
		for j, call := range m.ToolCalls {
			if out[i].ToolCalls[j].Arguments, err = t.maskJSON(ctx, call.Arguments); err != nil {
				return nil, nil, err
			}
		}
	}
	tools := append([]llm.ToolDef(nil), defs...)
	for i, d := range defs {
		var err error
		if tools[i].Description, err = t.maskText(ctx, d.Description, ""); err != nil {
			return nil, nil, err
		}
		if d.Parameters == nil {
			continue
		}
		data, err := json.Marshal(d.Parameters)
		if err != nil {
			return nil, nil, errors.New("invalid tool schema")
		}
		var schema map[string]any
		if json.Unmarshal(data, &schema) != nil {
			return nil, nil, errors.New("invalid tool schema")
		}
		if err = t.maskDescriptions(ctx, schema, 0); err != nil {
			return nil, nil, err
		}
		tools[i].Parameters = schema
	}
	// A block signed during an earlier, unprotected turn cannot be rewritten.
	// Stop rather than replay a now-protected value inside that signed block.
	for _, message := range out {
		for _, block := range message.ThinkingBlocks {
			if block.Signature == "" {
				continue
			}
			for _, value := range t.values {
				if strings.Contains(block.Thinking, value.Value) {
					return nil, nil, errors.New("signed thinking contains a protected value; start a new chat or use an explicit provider exception")
				}
			}
		}
	}
	return out, tools, nil
}

func (t *transaction) maskDescriptions(ctx context.Context, v any, depth int) error {
	if depth > 64 {
		return errors.New("tool schema nesting limit exceeded")
	}
	switch x := v.(type) {
	case map[string]any:
		for k, w := range x {
			if k == "description" {
				if text, ok := w.(string); ok {
					masked, err := t.maskText(ctx, text, "")
					if err != nil {
						return err
					}
					x[k] = masked
					continue
				}
			}
			if err := t.maskDescriptions(ctx, w, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, w := range x {
			if err := t.maskDescriptions(ctx, w, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

type restoreStream struct {
	tx      *transaction
	pending string
	emit    func(string) error
}

func (s *restoreStream) Write(chunk string) error {
	if s.emit == nil {
		return nil
	}
	s.pending += chunk
	cut := len(s.pending)
	if start := strings.LastIndex(s.pending, "{{"); start >= 0 {
		tail := s.pending[start:]
		upper := strings.ToUpper(strings.TrimLeftFunc(tail[2:], unicode.IsSpace))
		if len(tail) < 128 && !strings.Contains(tail, "}}") && (strings.HasPrefix("SB_", upper) || strings.HasPrefix(upper, "SB_")) {
			cut = start
		}
	}
	if cut > 0 && s.pending[cut-1] == '{' {
		cut--
	}
	for cut > 0 && !utf8.ValidString(s.pending[:cut]) {
		cut--
	}
	if cut == 0 {
		return nil
	}
	text, err := s.tx.restore(s.pending[:cut])
	if err != nil {
		return err
	}
	s.pending = s.pending[cut:]
	return s.emit(text)
}
func (s *restoreStream) Finish() error {
	if s.emit == nil || s.pending == "" {
		return nil
	}
	text, err := s.tx.restore(s.pending)
	s.pending = ""
	if err != nil {
		return err
	}
	return s.emit(text)
}
