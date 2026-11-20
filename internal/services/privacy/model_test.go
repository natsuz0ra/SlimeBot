package privacy

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in integration test with publisher model files, never a network download.
func TestLocalModelLive(t *testing.T) {
	source := os.Getenv("SLIMEBOT_PRIVACY_TEST_MODEL_DIR")
	if source == "" {
		t.Skip("set SLIMEBOT_PRIVACY_TEST_MODEL_DIR for pure Go model integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := t.TempDir()
	for _, f := range modelFiles {
		from, err := os.Open(filepath.Join(source, f.name))
		if err != nil {
			t.Fatal(err)
		}
		to, err := os.Create(filepath.Join(dir, f.name))
		if err != nil {
			from.Close()
			t.Fatal(err)
		}
		_, err = io.Copy(to, from)
		from.Close()
		to.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", "") // Inference must not spawn Python or any other executable.
	m := NewLocalModel(dir)
	defer m.Stop()
	if err := m.install(ctx); err != nil {
		t.Fatal(err)
	}
	text := "请联系李明，地址是北京市海淀区中关村大街27号1101室。"
	found, err := m.Detect(ctx, text, 0.8)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, v := range found {
		have[v.kind] = true
		if v.start < 0 || v.end > len(text) || v.start >= v.end {
			t.Fatal("invalid byte offsets")
		}
	}
	if !have["PERSON"] || !have["ADDRESS"] {
		t.Fatalf("missing local model categories: %v", have)
	}
	if _, err = m.Detect(ctx, "Please contact Alice Johnson at 123 Maple Street, Springfield, IL 62704.", 0.8); err != nil {
		t.Fatal(err)
	}
	if !m.Status().Running {
		t.Fatal("loaded model was not reused")
	}
	short, stop := context.WithCancel(ctx)
	stop()
	if _, err = m.Detect(short, text, 0.8); err == nil {
		t.Fatal("cancelled scan accepted")
	}
	deadline, stop := context.WithTimeout(ctx, time.Millisecond)
	defer stop()
	if _, err = m.Detect(deadline, strings.Repeat(text, 100), .8); err == nil {
		t.Fatal("scan ignored deadline")
	}
	// A cancelled scan must leave the shared model usable for the next request.
	if _, err = m.Detect(ctx, text, .8); err != nil {
		t.Fatal(err)
	}
	m.Stop()
	if m.Status().Running {
		t.Fatal("stopped model remains loaded")
	}
	if _, err = m.Detect(ctx, text, .8); err != nil {
		t.Fatal(err)
	}
	if err = m.Remove(); err != nil {
		t.Fatal(err)
	}
	if m.Status().Installed || m.Status().Running {
		t.Fatal("model removal did not release files and memory")
	}
}

func TestMissingModelIsAnError(t *testing.T) {
	m := NewLocalModel(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := m.Detect(ctx, "a synthetic input", 0.8); err == nil {
		t.Fatal("missing model accepted")
	}
}
