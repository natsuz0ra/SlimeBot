package inference

import (
	"context"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Finding struct {
	Start, End int
	Kind       string
	Score      float64
}
type prediction struct {
	token         Token
	label, margin int
	score         float64
}

// Publisher label pairs use BIO ordering: B then I, with label zero as O.
var categories = [11]string{"", "EMAIL", "PHONE", "ACCOUNT", "BANK_CARD", "", "", "SECRET", "ADDRESS", "", "PERSON"}

func (m *Model) Detect(ctx context.Context, text string, threshold float64) ([]Finding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !utf8.ValidString(text) || len(text) > 1<<20 {
		return nil, errModel
	}
	encoding := m.tokenizer.encode(text, true)
	tokens := encoding[1 : len(encoding)-1]
	selected := make(map[[2]int]prediction, len(tokens))
	for start := 0; start < len(tokens); start += 414 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(start+510, len(tokens))
		window := make([]Token, 0, end-start+2)
		window = append(window, Token{ID: 0})
		window = append(window, tokens[start:end]...)
		window = append(window, Token{ID: 2})
		ids := make([]int, len(window))
		for i, t := range window {
			ids[i] = t.ID
		}
		logits, err := m.Logits(ctx, ids)
		if err != nil {
			return nil, err
		}
		for i, t := range window {
			if t.Start == t.End {
				continue
			}
			row := logits[i*labels : (i+1)*labels]
			label := 0
			for j, v := range row {
				if v > row[label] {
					label = j
				}
			}
			var denominator float64
			for _, v := range row {
				denominator += math.Exp(float64(v - row[label]))
			}
			p := prediction{t, label, min(i, len(window)-1-i), 1 / denominator}
			key := [2]int{t.Start, t.End}
			if old, ok := selected[key]; !ok || p.margin > old.margin {
				selected[key] = p
			}
		}
		if end == len(tokens) {
			break
		}
	}
	ordered := make([]prediction, 0, len(selected))
	for _, p := range selected {
		ordered = append(ordered, p)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].token.Start == ordered[j].token.Start {
			return ordered[i].token.End < ordered[j].token.End
		}
		return ordered[i].token.Start < ordered[j].token.Start
	})
	var out []Finding
	var current *Finding
	finish := func() {
		if current != nil && current.Score >= threshold {
			// A composed character can map to its first source rune. Include any
			// trailing combining marks so the redacted original remains well formed.
			for current.End < len(text) {
				r, n := utf8.DecodeRuneInString(text[current.End:])
				if !unicode.IsMark(r) {
					break
				}
				current.End += n
			}
			out = append(out, *current)
		}
	}
	for _, p := range ordered {
		kind := categories[(p.label+1)/2]
		start, end := p.token.Start, p.token.End
		continuation := current != nil && kind != "" && kind == current.Kind && p.label%2 == 0 && start >= current.Start && (start <= current.End || strings.TrimSpace(text[current.End:start]) == "")
		if continuation {
			current.End = max(current.End, end)
			current.Score = min(current.Score, p.score)
		} else {
			finish()
			current = nil
			if kind != "" {
				current = &Finding{start, end, kind, p.score}
			}
		}
	}
	finish()
	return out, ctx.Err()
}
