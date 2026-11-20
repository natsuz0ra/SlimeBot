package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPublisherReference(t *testing.T) {
	dir := os.Getenv("SLIMEBOT_PRIVACY_TEST_MODEL_DIR")
	if dir == "" {
		t.Skip("set SLIMEBOT_PRIVACY_TEST_MODEL_DIR to test publisher files offline")
	}
	data, err := os.ReadFile("testdata/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Text     string
		Tokens   []Token
		Logits   [][]float32
		Findings []Finding
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	start := time.Now()
	m, err := Load(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("load", time.Since(start))
	for index, c := range cases {
		t.Run(fmt.Sprintf("case_%03d", index), func(t *testing.T) {
			tokens := m.tokenizer.Encode(c.Text)
			covered := m.tokenizer.encode(c.Text, true)
			if len(covered) != len(tokens) {
				t.Fatal("source coverage changed tokenization")
			}
			for i, tok := range covered {
				if tok.ID != tokens[i].ID || tok.Start < 0 || tok.End > len(c.Text) || tok.Start > tok.End {
					t.Fatal("invalid source coverage")
				}
			}
			if c.Tokens != nil && !reflect.DeepEqual(tokens, c.Tokens) {
				t.Errorf("tokens\n got %v\nwant %v", tokens, c.Tokens)
			}
			if c.Findings != nil {
				found, err := m.Detect(ctx, c.Text, 0.8)
				if err != nil {
					t.Fatal(err)
				}
				if len(found) != len(c.Findings) {
					t.Fatalf("findings %v, want %v", found, c.Findings)
				}
				for i, want := range c.Findings {
					got := found[i]
					if got.Start != want.Start || got.End != want.End || got.Kind != want.Kind || math.Abs(got.Score-want.Score) > 0.05 {
						t.Errorf("finding %v, want %v", got, want)
					}
				}
			}
			if c.Logits == nil {
				return
			}
			ids := make([]int, len(tokens))
			for i, tok := range tokens {
				ids[i] = tok.ID
			}
			start := time.Now()
			got, err := m.Logits(ctx, ids)
			if err != nil {
				t.Fatal(err)
			}
			var maxError, sum float64
			disagreements := 0
			for i, row := range c.Logits {
				bestGot, bestWant := 0, 0
				for j, want := range row {
					if math.IsNaN(float64(got[i*labels+j])) || math.IsInf(float64(got[i*labels+j]), 0) {
						t.Fatal("nonfinite logit")
					}
					d := math.Abs(float64(got[i*labels+j] - want))
					maxError = max(maxError, d)
					sum += d
					if got[i*labels+j] > got[i*labels+bestGot] {
						bestGot = j
					}
					if want > row[bestWant] {
						bestWant = j
					}
				}
				if bestGot != bestWant {
					disagreements++
				}
			}
			t.Logf("%d tokens %s max error %.6f mean %.6f classifications differing %d", len(ids), time.Since(start), maxError, sum/float64(len(got)), disagreements)
			if maxError > 0.5 || sum/float64(len(got)) > 0.1 || disagreements > 0 {
				t.Fatal("inference differs from reference")
			}
		})
	}
	for _, prefix := range []string{"\x1e", "\x1e\x1e"} {
		text := prefix + "请联系李明，地址是北京市海淀区中关村大街27号1101室。"
		found, err := m.Detect(ctx, text, .8)
		if err != nil {
			t.Fatal(err)
		}
		have := false
		for _, v := range found {
			if v.Kind == "PERSON" && text[v.Start:v.End] == "李明" {
				have = true
			}
		}
		if !have {
			t.Fatalf("removed leading controls shifted entity offsets: %v", found)
		}
	}
	covered := m.tokenizer.encode("e\u0301", true)
	if len(covered) != 3 || covered[1].Start != 0 || covered[1].End != 3 {
		t.Fatal("combined source character was partially covered")
	}
}

func TestUnigramPathsAndOffsets(t *testing.T) {
	tokenizer := &Tokenizer{root: &pieceNode{id: -1}, chars: []uint32{0}, unknownScore: -20}
	// Ambiguous segmentation must maximize the full path, not greedily choose
	// the longest prefix. Include a fused unknown run and byte offsets after CJK.
	for id, p := range []struct {
		piece string
		score float64
	}{{"▁", -1}, {"a", -1}, {"b", -1}, {"ab", -5}, {"中", -1}} {
		node := tokenizer.root
		for _, v := range []byte(p.piece) {
			if node.next == nil {
				node.next = map[byte]*pieceNode{}
			}
			if node.next[v] == nil {
				node.next[v] = &pieceNode{id: -1}
			}
			node = node.next[v]
		}
		node.id = id + 10
		node.score = p.score
	}
	got := tokenizer.Encode("ab 中λμ")
	want := []Token{{0, 0, 0}, {10, 0, 1}, {11, 0, 1}, {12, 1, 2}, {10, 3, 6}, {14, 3, 6}, {3, 6, 10}, {2, 0, 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func BenchmarkGuard(b *testing.B) {
	dir := os.Getenv("SLIMEBOT_PRIVACY_TEST_MODEL_DIR")
	if dir == "" {
		b.Skip("set model directory")
	}
	m, err := Load(context.Background(), dir)
	if err != nil {
		b.Fatal(err)
	}
	for name, text := range map[string]string{"short": "请联系李明，地址是北京市海淀区中关村大街27号1101室。", "long": strings.Repeat("Documentation. ", 165) + "Contact Alice Johnson at 123 Maple Street, Springfield, IL 62704."} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := m.Detect(context.Background(), text, .8); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestQuantize(t *testing.T) {
	q, scale := quantize([]float32{-1, 0, 1})
	if scale <= 0 || q[1] != 0 || q[0] >= 0 || q[2] <= 0 {
		t.Fatalf("invalid centered quantization: %v %v", q, scale)
	}
	q, scale = quantize([]float32{0, 0})
	if scale != 1 || q[0] != 0 || q[1] != 0 {
		t.Fatal("constant quantization")
	}
}

func TestMalformedProtobuf(t *testing.T) {
	for _, b := range [][]byte{{0}, {0x3a, 0xff}, {0x3d, 0}, {0x3b}, {0x3a, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}} {
		if walk(b, func(field) error { return nil }) == nil {
			t.Errorf("accepted %x", b)
		}
	}
}
