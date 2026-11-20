package inference

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// Token offsets are UTF-8 byte offsets into the unchanged input, never into a
// normalized copy. Synthetic start/end tokens have zero-length offsets.
type Token struct{ ID, Start, End int }
type mappedRune struct {
	r          rune
	start, end int
}
type pieceNode struct {
	next  map[byte]*pieceNode
	id    int
	score float64
}
type Tokenizer struct {
	root         *pieceNode
	unknownScore float64
	chars        []uint32
	replacements string
}

func LoadTokenizer(dir string) (*Tokenizer, error) {
	data, err := os.ReadFile(filepath.Join(dir, "tokenizer.json"))
	if err != nil || len(data) > 4<<20 {
		return nil, errModel
	}
	var cfg struct {
		Normalizer struct {
			Normalizers []struct {
				Map string `json:"precompiled_charsmap"`
			} `json:"normalizers"`
		} `json:"normalizer"`
		Model struct {
			Type         string              `json:"type"`
			Unk          int                 `json:"unk_id"`
			Vocab        [][]json.RawMessage `json:"vocab"`
			ByteFallback bool                `json:"byte_fallback"`
		} `json:"model"`
	}
	if json.Unmarshal(data, &cfg) != nil || cfg.Model.Type != "Unigram" || cfg.Model.Unk != 3 || cfg.Model.ByteFallback || len(cfg.Model.Vocab) != 40003 || len(cfg.Normalizer.Normalizers) != 1 {
		return nil, errModel
	}
	blob, err := base64.StdEncoding.DecodeString(cfg.Normalizer.Normalizers[0].Map)
	if err != nil || len(blob) < 8 {
		return nil, errModel
	}
	n := int(binary.LittleEndian.Uint32(blob))
	if n < 1024 || n%1024 != 0 || n >= len(blob)-4 {
		return nil, errModel
	}
	t := &Tokenizer{root: &pieceNode{id: -1}, replacements: string(blob[4+n:]), chars: make([]uint32, n/4), unknownScore: math.Inf(1)}
	if !utf8.ValidString(t.replacements) || !strings.HasSuffix(t.replacements, "\x00") {
		return nil, errModel
	}
	for i := range t.chars {
		t.chars[i] = binary.LittleEndian.Uint32(blob[4+i*4:])
	}
	for id, pair := range cfg.Model.Vocab {
		if len(pair) != 2 {
			return nil, errModel
		}
		var text string
		var score float64
		if json.Unmarshal(pair[0], &text) != nil || json.Unmarshal(pair[1], &score) != nil || text == "" || math.IsNaN(score) || math.IsInf(score, 0) {
			return nil, errModel
		}
		t.unknownScore = min(t.unknownScore, score)
		node := t.root
		for i := 0; i < len(text); i++ {
			if node.next == nil {
				node.next = map[byte]*pieceNode{}
			}
			next := node.next[text[i]]
			if next == nil {
				next = &pieceNode{id: -1}
				node.next[text[i]] = next
			}
			node = next
		}
		node.id = id
		node.score = score
	}
	t.unknownScore -= 10
	return t, nil
}

// The SentencePiece charsmap is a little-endian XOR double-array trie followed
// by NUL-terminated UTF-8 replacements. Use the first terminal prefix, matching
// the publisher's Precompiled normalizer; bounds checks also cover corrupt tries.
func (t *Tokenizer) replacement(s string) (string, bool) {
	index := uint32(0)
	for i := 0; i < len(s) && s[i] != 0; i++ {
		if uint64(index) >= uint64(len(t.chars)) {
			return "", false
		}
		unit := t.chars[index]
		offset := (unit >> 10) << ((unit >> 6) & 8)
		index ^= offset ^ uint32(s[i])
		if uint64(index) >= uint64(len(t.chars)) {
			return "", false
		}
		unit = t.chars[index]
		if unit&0x800000ff != uint32(s[i]) {
			return "", false
		}
		if unit&0x100 != 0 {
			leaf := index ^ ((unit >> 10) << ((unit >> 6) & 8))
			if uint64(leaf) >= uint64(len(t.chars)) {
				return "", false
			}
			value := int(t.chars[leaf] & 0x7fffffff)
			if value >= len(t.replacements) {
				return "", false
			}
			end := strings.IndexByte(t.replacements[value:], 0)
			if end < 0 {
				return "", false
			}
			return t.replacements[value : value+end], true
		}
	}
	return "", false
}

func originalRunes(s string, offset int) []mappedRune {
	out := make([]mappedRune, 0, utf8.RuneCountInString(s))
	for i, r := range s {
		out = append(out, mappedRune{r, offset + i, offset + i + utf8.RuneLen(r)})
	}
	return out
}

func (t *Tokenizer) normalize(s string, offset int, cover bool) []mappedRune {
	type edit struct {
		r      rune
		change int
	}
	var edits []edit
	var covered []mappedRune
	replace := func(original string, replacement string, at int) {
		if cover {
			source := originalRunes(original, offset+at)
			added := []rune(replacement)
			for i, r := range added {
				v := source[min(i, len(source)-1)]
				v.r = r
				if i == len(added)-1 {
					v.end = source[len(source)-1].end
				}
				covered = append(covered, v)
			}
			if len(added) == 0 && len(covered) > 0 {
				covered[len(covered)-1].end = offset + at + len(original)
			}
			return
		}
		count := utf8.RuneCountInString(original)
		added := []rune(replacement)
		for _, r := range added {
			edits = append(edits, edit{r, 0})
		}
		difference := len(added) - count
		if difference > 0 {
			for i := len(edits) - difference; i < len(edits); i++ {
				edits[i].change = 1
			}
		} else if difference < 0 && len(edits) > 0 {
			edits[len(edits)-1].change += difference
		}
	}
	graphemes := uniseg.NewGraphemes(s)
	for graphemes.Next() {
		part := graphemes.Str()
		at, _ := graphemes.Positions()
		if len(part) < 6 {
			if replacement, ok := t.replacement(part); ok {
				replace(part, replacement, at)
				continue
			}
		}
		for index, r := range part {
			piece := string(r)
			if replacement, ok := t.replacement(piece); ok {
				replace(piece, replacement, at+index)
			} else {
				if cover {
					covered = append(covered, mappedRune{r, offset + at + index, offset + at + index + len(piece)})
				} else {
					edits = append(edits, edit{r, 0})
				}
			}
		}
	}
	if cover {
		return covered
	}
	original := originalRunes(s, offset)
	out := make([]mappedRune, 0, len(edits))
	cursor := 0
	for _, e := range edits {
		source := cursor
		if e.change > 0 {
			source = max(0, cursor-1)
		} else {
			cursor += 1 - e.change
		}
		source = min(source, len(original)-1)
		if source >= 0 {
			r := original[source]
			r.r = e.r
			out = append(out, r)
		}
	}
	return out
}

func (t *Tokenizer) segment(runes []mappedRune) []Token {
	if len(runes) == 0 {
		return nil
	}
	if runes[0].r != '▁' {
		source := runes[0]
		source.r = '▁'
		runes = append([]mappedRune{source}, runes...)
	}
	var b strings.Builder
	var starts, ends []int
	for _, r := range runes {
		s := string(r.r)
		b.WriteString(s)
		for range len(s) {
			starts = append(starts, r.start)
			ends = append(ends, r.end)
		}
	}
	s := b.String()
	scores := make([]float64, len(s)+1)
	prev := make([]int, len(s)+1)
	ids := make([]int, len(s)+1)
	for i := 1; i < len(scores); i++ {
		scores[i] = math.Inf(-1)
	}
	for start := 0; start < len(s); {
		_, size := utf8.DecodeRuneInString(s[start:])
		single := false
		update := func(end, id int, score float64) {
			score += scores[start]
			if score > scores[end] {
				scores[end] = score
				prev[end] = start
				ids[end] = id
			}
		}
		node := t.root
		for end := start; end < len(s); end++ {
			node = node.next[s[end]]
			if node == nil {
				break
			}
			if node.id >= 0 {
				update(end+1, node.id, node.score)
				single = single || end+1-start == size
			}
		}
		if !single {
			update(start+size, 3, t.unknownScore)
		}
		start += size
	}
	var reverse []Token
	for end := len(s); end > 0; {
		start := prev[end]
		id := ids[end]
		if id == 3 && len(reverse) > 0 && reverse[len(reverse)-1].ID == 3 {
			reverse[len(reverse)-1].Start = starts[start]
		} else {
			reverse = append(reverse, Token{id, starts[start], ends[end-1]})
		}
		end = start
	}
	slices.Reverse(reverse)
	return reverse
}

func (t *Tokenizer) ordinary(s string, offset int, cover bool) []Token {
	runes := t.normalize(s, offset, cover)
	var out []Token
	for i := 0; i < len(runes); {
		if unicode.IsSpace(runes[i].r) {
			end := i + 1
			for end < len(runes) && unicode.IsSpace(runes[end].r) {
				end++
			}
			// The first publisher split removes whitespace except after ASCII
			// digits. The second isolates each remaining whitespace character.
			if i > 0 && runes[i-1].r >= '0' && runes[i-1].r <= '9' {
				for _, r := range runes[i:end] {
					r.r = '▁'
					out = append(out, t.segment([]mappedRune{r})...)
				}
			}
			i = end
			continue
		}
		end := i + 1
		for end < len(runes) && !unicode.IsSpace(runes[end].r) && runes[end].r != '▁' {
			end++
		}
		out = append(out, t.segment(runes[i:end])...)
		i = end
	}
	return out
}

// Encode handles the five added special tokens before normalization, then
// applies whitespace splitting, Metaspace and maximum-score Unigram decoding.
func (t *Tokenizer) Encode(text string) []Token {
	return t.encode(text, false)
}

// Privacy inference keeps conservative source coverage in addition to the
// reference tokenizer's token IDs. In particular, a removed leading control
// character must not shift every later entity back by one source character.
// Contracted combining sequences are covered in full when redacting originals.
func (t *Tokenizer) encode(text string, cover bool) []Token {
	out := []Token{{ID: 0}}
	special := []struct {
		text string
		id   int
	}{{"<mask>", 40002}, {"<pad>", 1}, {"<unk>", 3}, {"</s>", 2}, {"<s>", 0}}
	for offset := 0; offset < len(text); {
		next := len(text)
		which := -1
		for i, s := range special {
			if at := strings.Index(text[offset:], s.text); at >= 0 && offset+at < next {
				next = offset + at
				which = i
			}
		}
		end := next
		start := next
		if which >= 0 && special[which].id == 40002 {
			for end > offset {
				r, n := utf8.DecodeLastRuneInString(text[offset:end])
				if !unicode.IsSpace(r) {
					break
				}
				end -= n
			}
			start = end
		}
		out = append(out, t.ordinary(text[offset:end], offset, cover)...)
		if which < 0 {
			break
		}
		s := special[which]
		out = append(out, Token{s.id, start, next + len(s.text)})
		offset = next + len(s.text)
	}
	return append(out, Token{ID: 2})
}
