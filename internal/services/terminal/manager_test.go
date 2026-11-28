package terminal

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestReplayWrapsAndKeepsNewestBytes(t *testing.T) {
	var ring ringBuffer
	ring.append(bytes.Repeat([]byte("a"), replaySize-2))
	ring.append([]byte("last"))
	if got := ring.snapshot(); len(got) != replaySize || !bytes.HasSuffix(got, []byte("last")) {
		t.Fatalf("invalid wrapped snapshot length %d", len(got))
	}
	newest := bytes.Repeat([]byte("z"), replaySize+10)
	ring.append(newest)
	if got := ring.snapshot(); !bytes.Equal(got, newest[10:]) {
		t.Fatal("oversized write did not retain newest bytes")
	}
}

func TestPTYInputResizeExitAndReplay(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	defer m.CloseAll(ctx)
	shell := "/bin/sh"
	command := "printf 'SLIMEBOT_%s\\n' PTY_OK; exit 7\n"
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
		command = "echo SLIMEBOT_%OS%_PTY_OK\rexit /b 7\r"
	}
	info, err := m.Create("owner", Options{Shell: shell, Cwd: t.TempDir()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get("other", info.ID); err != ErrNotFound {
		t.Fatalf("access by other owner: %v", err)
	}
	s, err := m.Get("owner", info.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, initial, output, unsubscribe := s.Subscribe()
	defer unsubscribe()
	if err := s.Resize(100, 28); err != nil {
		t.Fatal(err)
	}
	if err := s.Write([]byte(command)); err != nil {
		t.Fatal(err)
	}
	got := string(initial)
	for output != nil {
		select {
		case data, ok := <-output:
			if !ok {
				output = nil
			}
			got += string(data)
		case <-ctx.Done():
			t.Fatalf("shell didn't exit: %q", got)
		}
	}
	end, replay, _, detach := s.Subscribe()
	defer detach()
	marker := "SLIMEBOT_PTY_OK"
	if runtime.GOOS == "windows" {
		marker = "SLIMEBOT_Windows_NT_PTY_OK"
	}
	if end.ExitCode == nil || *end.ExitCode != 7 || end.Cols != 100 || !strings.Contains(got, marker) || !bytes.Contains(replay, []byte(marker)) {
		t.Fatalf("exit info %#v, output %q", end, got)
	}
}

func TestFilteredTerminalEnvStripsInternalSecrets(t *testing.T) {
	env := filteredTerminalEnv([]string{
		"PATH=/usr/bin",
		"JWT_SECRET=top-secret",
		"slimebot_cli_token=cli-secret",
		"SLIMEBOT_DESKTOP_TOKEN=desk-secret",
		"TERM=dumb",
	})
	joined := strings.Join(env, "\n")
	for _, secret := range []string{"JWT_SECRET=", "cli-secret", "desk-secret", "TERM=dumb"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("unexpected secret or stale TERM in env: %q", joined)
		}
	}
	if !strings.Contains(joined, "TERM=xterm-256color") || !strings.Contains(joined, "COLORTERM=truecolor") || !strings.Contains(joined, "PATH=/usr/bin") {
		t.Fatalf("missing expected terminal env entries: %q", joined)
	}
	if got := trimExeSuffix("PWSH.EXE"); got != "PWSH" {
		t.Fatalf("trimExeSuffix(PWSH.EXE) = %q, want PWSH", got)
	}
}
