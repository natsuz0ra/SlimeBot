package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
)

func TestInitServerKeepsConsoleWhenLogDirectoryFails(t *testing.T) {
	blockingFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var console bytes.Buffer
	logger, cleanup, err := Init(Options{
		Mode:          ModeServer,
		LogDir:        blockingFile,
		ConsoleWriter: &console,
		Now:           func() time.Time { return time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC) },
		Level:         zapcore.InfoLevel,
	})
	if err != nil {
		t.Fatalf("server console should survive file logging failure: %v", err)
	}
	defer cleanup()
	if !strings.Contains(console.String(), "file_logging_disabled") || !strings.Contains(console.String(), "create log directory") {
		t.Fatalf("missing explicit file logging warning: %s", console.String())
	}
	logger.Info("server_still_visible")
	if !strings.Contains(console.String(), "server_still_visible") {
		t.Fatalf("server console output was lost: %s", console.String())
	}
}

func TestInitCLIReturnsErrorWhenNoCoreCanBeCreated(t *testing.T) {
	blockingFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := Init(Options{Mode: ModeCLI, LogDir: blockingFile})
	defer cleanup()
	if err == nil || !strings.Contains(err.Error(), "create log directory") {
		t.Fatalf("Init() error = %v, want real directory error", err)
	}
}

func TestInitDisableFileSuppressesFileAndKeepsServerConsole(t *testing.T) {
	dir := t.TempDir()
	var console bytes.Buffer
	logger, cleanup, err := Init(Options{
		Mode:          ModeServer,
		LogDir:        dir,
		DisableFile:   true,
		ConsoleWriter: &console,
		Now:           func() time.Time { return time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	logger.Info("console_only")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("DisableFile created filesystem entries: %+v", entries)
	}
	if !strings.Contains(console.String(), "console_only") {
		t.Fatalf("console output missing: %s", console.String())
	}
}
