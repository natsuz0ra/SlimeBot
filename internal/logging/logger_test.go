package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
)

func TestInitCLIModeNoConsoleOutput(t *testing.T) {
	dir := t.TempDir()
	var console bytes.Buffer
	logger, cleanup, err := Init(Options{
		Mode:          ModeCLI,
		LogDir:        dir,
		Now:           func() time.Time { return time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC) },
		ConsoleWriter: &console,
		Level:         zapcore.InfoLevel,
	})
	if err != nil {
		t.Fatalf("init logger failed: %v", err)
	}
	defer cleanup()
	logger.Info("cli_test")
	cleanup()

	if console.Len() != 0 {
		t.Fatalf("cli mode should not write to console")
	}
	if _, err := os.Stat(filepath.Join(dir, "cli-20260404.log")); err != nil {
		t.Fatalf("expected file log exists: %v", err)
	}
}

func TestInitServerModeWithConsoleOutput(t *testing.T) {
	dir := t.TempDir()
	var console bytes.Buffer
	logger, cleanup, err := Init(Options{
		Mode:          ModeServer,
		LogDir:        dir,
		Now:           func() time.Time { return time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC) },
		ConsoleWriter: &console,
		Level:         zapcore.InfoLevel,
	})
	if err != nil {
		t.Fatalf("init logger failed: %v", err)
	}
	defer cleanup()
	logger.Info("server_test")
	cleanup()

	if console.Len() == 0 {
		t.Fatalf("server mode should write to console")
	}
	if _, err := os.Stat(filepath.Join(dir, "server-20260404.log")); err != nil {
		t.Fatalf("expected file log exists: %v", err)
	}
}

func TestCleanupOldLogs(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		p := filepath.Join(dir, "x"+time.Duration(i).String()+".log")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := now.AddDate(0, 0, -i)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	if err := cleanupOldLogs(dir, now, 14, 20); err != nil {
		t.Fatalf("cleanup logs failed: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 14 {
		t.Fatalf("expected at most 14 files, got %d", len(entries))
	}
}
