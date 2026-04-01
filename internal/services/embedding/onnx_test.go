package embedding

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestMeanPoolNormalize(t *testing.T) {
	// batch=1, seq=2, hidden=2
	tokenEmbeddings := []float32{
		1, 2,
		3, 4,
	}
	attentionMask := []int64{1, 0}
	vectors := meanPoolNormalize(tokenEmbeddings, attentionMask, 1, 2, 2)
	if len(vectors) != 1 || len(vectors[0]) != 2 {
		t.Fatalf("unexpected vector shape: %#v", vectors)
	}
	want0 := float32(1.0 / math.Sqrt(5.0))
	want1 := float32(2.0 / math.Sqrt(5.0))
	if diff := float32(math.Abs(float64(vectors[0][0] - want0))); diff > 1e-6 {
		t.Fatalf("unexpected vectors[0][0]: got=%v want=%v", vectors[0][0], want0)
	}
	if diff := float32(math.Abs(float64(vectors[0][1] - want1))); diff > 1e-6 {
		t.Fatalf("unexpected vectors[0][1]: got=%v want=%v", vectors[0][1], want1)
	}
}

func TestResolveTokenizerJSONPath(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/tokenizer.json"
	if err := osWriteFile(path, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	got, err := resolveTokenizerJSONPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(path) {
		t.Fatalf("unexpected path: got=%s want=%s", got, path)
	}
}

func osWriteFile(path string, content []byte) error {
	return os.WriteFile(path, content, 0o644)
}
