package privacy

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"slimebot/internal/services/llm"
)

var ErrBlocked = errors.New("privacy policy blocked this request; nothing was sent")
var placeholderRE = regexp.MustCompile(`(?i)\{\{\s*(SB_[A-Z0-9_]{1,24}_[0-9a-f]{32})\s*\}\}`)
var unresolvedRE = regexp.MustCompile(`(?i)(?:\{\{\s*)?SB_[A-Z0-9_]+_[0-9a-f]{8,}`)

type transaction struct {
	service         *Service
	config          Config
	key             []byte
	scope           llm.PrivacyScope
	values          map[string]entry
	index           map[string][]entry
	hits            []Hit
	record          Record
	preview         bool
	suppressRecord  bool
	processed       int
	modelCache      map[string][]span
	scanDuration    time.Duration
	scanComplete    bool
	restoreDuration time.Duration
	configHash      [32]byte
}

func (t *transaction) indexValue(v entry) {
	if len(v.Value) >= 8 {
		head := v.Value[:8]
		for _, e := range t.index[head] {
			if e == v {
				return
			}
		}
		t.index[head] = append(t.index[head], v)
	}
}

func (t *transaction) placeholder(kind, value string) (string, error) {
	for nonce := 0; nonce < 4; nonce++ {
		h := hmac.New(sha256.New, t.key)
		fmt.Fprintf(h, "%s\x00%s\x00%d\x00%s", t.scope.SessionID, kind, nonce, value)
		p := "{{SB_" + kind + "_" + hex.EncodeToString(h.Sum(nil)[:16]) + "}}"
		if old, ok := t.values[p]; !ok || old.Value == value {
			v := entry{value, kind}
			t.values[p] = v
			t.indexValue(v)
			return p, nil
		}
	}
	return "", errors.New("privacy placeholder collision")
}

func (t *transaction) maskText(ctx context.Context, text, field string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(text) > 4<<20 || t.processed+len(text) > 32<<20 {
		return "", errors.New("privacy input exceeds the text limit")
	}
	t.processed += len(text)
	if text == "" {
		return text, nil
	}
	// Structured content is inspected as values, so a phone inside a string is
	// protected without changing numeric types, object keys or encoded payloads.
	trimmed := strings.TrimSpace(text)
	if (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) && json.Valid([]byte(text)) {
		return t.maskJSON(ctx, text)
	}
	found := t.ruleCandidates(text)
	if t.config.Secrets && secretField(field) && realSecret(text) {
		found = append(found, span{0, len(text), "SECRET", "field"})
	}
	for i := 0; i+8 <= len(text); i++ {
		for _, v := range t.index[text[i:i+8]] {
			if t.config.covers(v.Kind) && strings.HasPrefix(text[i:], v.Value) {
				found = append(found, span{i, i + len(v.Value), v.Kind, "known"})
			}
		}
	}
	if t.config.ModelEnabled {
		if !utf8.ValidString(text) {
			return "", errors.New("local model requires valid UTF-8 text; nothing was sent")
		}
		if t.modelCache == nil {
			t.modelCache = map[string][]span{}
		}
		more, ok := t.modelCache[text]
		if !ok {
			var err error
			more, err = t.service.detect(ctx, text, t.config.ModelThreshold)
			if err != nil {
				return "", errors.New("local privacy model could not complete scanning; nothing was sent")
			}
			t.modelCache[text] = more
		}
		for _, v := range more {
			if v.start >= 0 && v.end <= len(text) && v.start < v.end && t.config.covers(v.kind) {
				v.source = "model"
				if v.kind == "EMAIL" && !realEmail(text[v.start:v.end]) {
					continue
				}
				found = append(found, v)
			}
		}
	}
	protected := placeholderRE.FindAllStringIndex(text, -1)
	var allowRanges [][2]int
	for _, f := range found {
		if t.config.allows(text[f.start:f.end]) {
			allowRanges = append(allowRanges, [2]int{f.start, f.end})
		}
	}
	allowed := func(f span) bool {
		for _, r := range allowRanges {
			if f.start >= r[0] && f.end <= r[1] {
				return true
			}
		}
		return false
	}
	chosen := ordered(found)
	type matchID struct {
		start, end int
		kind       string
	}
	selected := map[matchID]bool{}
	for _, f := range chosen {
		selected[matchID{f.start, f.end, f.kind}] = true
	}
	noted := map[matchID]bool{}
	for _, f := range found {
		if t.config.action(f.kind) != Block || allowed(f) {
			continue
		}
		inside := false
		for _, p := range protected {
			if f.start < p[1] && f.end > p[0] {
				inside = true
				break
			}
		}
		if inside {
			continue
		}
		t.record.Blocked = true
		id := matchID{f.start, f.end, f.kind}
		if !selected[id] && !noted[id] {
			t.hits = append(t.hits, Hit{f.kind, f.source, Block, f.start, f.end, ""})
			t.record.Kinds[f.kind]++
			noted[id] = true
		}
	}
	var out strings.Builder
	last := 0
	for _, f := range chosen {
		if f.start < last || f.start >= f.end || allowed(f) {
			continue
		}
		overlap := false
		for _, p := range protected {
			if f.start < p[1] && f.end > p[0] {
				overlap = true
				break
			}
		}
		if overlap {
			continue
		}
		value := text[f.start:f.end]
		if len(value) > 64<<10 {
			return "", errors.New("sensitive value exceeds privacy mapping limit; nothing was sent")
		}
		action := t.config.action(f.kind)
		p := ""
		if action != Warn {
			var err error
			p, err = t.placeholder(f.kind, value)
			if err != nil {
				return "", err
			}
		}
		t.hits = append(t.hits, Hit{f.kind, f.source, action, f.start, f.end, p})
		t.record.Kinds[f.kind]++
		out.WriteString(text[last:f.start])
		if action == Warn {
			out.WriteString(value)
			t.record.Warned++
		} else {
			out.WriteString(p)
			t.record.Masked++
			if action == Block {
				t.record.Blocked = true
			}
		}
		last = f.end
	}
	if last == 0 {
		return text, nil
	}
	out.WriteString(text[last:])
	return out.String(), nil
}

func (t *transaction) ruleCandidates(text string) []span {
	h := sha256.New()
	h.Write(t.configHash[:])
	h.Write([]byte(text))
	var key [32]byte
	copy(key[:], h.Sum(nil))
	s := t.service
	s.mu.Lock()
	cached, ok := s.ruleCache[key]
	s.mu.Unlock()
	if ok {
		return append([]span(nil), cached...)
	}
	found := candidates(text, t.config)
	// Only offsets and categories are retained. Neither text nor values enter
	// this bounded cache, and known-value matching is always performed afresh.
	if len(found) <= 128 {
		s.mu.Lock()
		if len(s.ruleCache) >= 128 {
			s.ruleCache = map[[32]byte][]span{}
		}
		s.ruleCache[key] = append([]span{}, found...)
		s.mu.Unlock()
	}
	return found
}

func decodeJSON(raw string) (any, error) {
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("invalid JSON tail")
	}
	return v, nil
}

func (t *transaction) maskJSON(ctx context.Context, raw string) (string, error) {
	v, err := decodeJSON(raw)
	if err != nil {
		return "", errors.New("invalid tool arguments JSON")
	}
	before := len(t.hits)
	var walk func(any, string, int) (any, error)
	walk = func(v any, key string, depth int) (any, error) {
		if depth > 64 {
			return nil, errors.New("privacy JSON nesting limit exceeded")
		}
		switch x := v.(type) {
		case string:
			if strings.HasPrefix(x, "data:") {
				return x, nil
			}
			return t.maskText(ctx, x, key)
		case map[string]any:
			for k, w := range x {
				next, e := walk(w, k, depth+1)
				if e != nil {
					return nil, e
				}
				x[k] = next
			}
		case []any:
			for i, w := range x {
				next, e := walk(w, "", depth+1)
				if e != nil {
					return nil, e
				}
				x[i] = next
			}
		}
		return v, nil
	}
	v, err = walk(v, "", 0)
	if err != nil {
		return "", err
	}
	if before == len(t.hits) {
		return raw, nil
	}
	data, err := json.Marshal(v)
	return string(data), err
}

func (t *transaction) restore(text string) (string, error) {
	started := time.Now()
	defer func() { t.restoreDuration += time.Since(started) }()
	var restoreErr error
	unmatched := placeholderRE.ReplaceAllString(text, "")
	text = placeholderRE.ReplaceAllStringFunc(text, func(p string) string {
		body := placeholderRE.FindStringSubmatch(p)[1]
		cut := strings.LastIndex(body, "_")
		canonical := "{{" + strings.ToUpper(body[:cut]) + "_" + strings.ToLower(body[cut+1:]) + "}}"
		if v, ok := t.values[canonical]; ok {
			t.record.Restored++
			return v.Value
		}
		restoreErr = errors.New("a privacy placeholder could not be restored")
		return p
	})
	if restoreErr == nil && unresolvedRE.MatchString(unmatched) {
		restoreErr = errors.New("an incomplete privacy placeholder could not be restored")
	}
	return text, restoreErr
}

func (t *transaction) restoreJSON(raw string) (string, error) {
	v, err := decodeJSON(raw)
	if err != nil {
		return "", errors.New("model returned invalid tool arguments JSON")
	}
	var walk func(any, int) (any, error)
	walk = func(v any, depth int) (any, error) {
		if depth > 64 {
			return nil, errors.New("tool arguments nesting limit exceeded")
		}
		switch x := v.(type) {
		case string:
			return t.restore(x)
		case map[string]any:
			for k, w := range x {
				next, e := walk(w, depth+1)
				if e != nil {
					return nil, e
				}
				x[k] = next
			}
		case []any:
			for i, w := range x {
				next, e := walk(w, depth+1)
				if e != nil {
					return nil, e
				}
				x[i] = next
			}
		}
		return v, nil
	}
	v, err = walk(v, 0)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err = enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}
