package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildExecInvocationRequiresCommand(t *testing.T) {
	_, err := buildExecInvocation("linux", execRunConfig{command: "", shell: "auto"})
	if err == nil {
		t.Fatal("expected error when command is empty")
	}
	if !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildExecInvocationRejectsInvalidShell(t *testing.T) {
	_, err := buildExecInvocation("linux", execRunConfig{command: "echo ok", shell: "fish"})
	if err == nil {
		t.Fatal("expected invalid shell error")
	}
	if !strings.Contains(err.Error(), "invalid shell") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildExecInvocationAutoShellLinux(t *testing.T) {
	orig := execLookPath
	t.Cleanup(func() { execLookPath = orig })
	execLookPath = func(file string) (string, error) {
		if file == "bash" {
			return "/bin/bash", nil
		}
		return "", errors.New("not found")
	}

	inv, err := buildExecInvocation("linux", execRunConfig{command: "echo ok", shell: "auto"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.commandName != "bash" {
		t.Fatalf("expected bash for linux auto shell, got %s", inv.commandName)
	}
	if len(inv.commandArgs) != 2 || inv.commandArgs[0] != "-lc" {
		t.Fatalf("unexpected args: %#v", inv.commandArgs)
	}
}

func TestBuildExecInvocationAutoShellWindows(t *testing.T) {
	inv, err := buildExecInvocation("windows", execRunConfig{command: "echo ok", shell: "auto"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.commandName != "powershell" {
		t.Fatalf("expected powershell for windows auto shell, got %s", inv.commandName)
	}
	if len(inv.commandArgs) < 4 || inv.commandArgs[2] != "-Command" {
		t.Fatalf("unexpected args: %#v", inv.commandArgs)
	}
}

func TestResolveTimeoutMsDefaultAndClamp(t *testing.T) {
	if got := resolveTimeoutMs(""); got != 30000 {
		t.Fatalf("expected default 30000, got %d", got)
	}
	if got := resolveTimeoutMs("700000"); got != 600000 {
		t.Fatalf("expected clamp to 600000, got %d", got)
	}
	if got := resolveTimeoutMs("1"); got != 1 {
		t.Fatalf("expected parsed timeout 1, got %d", got)
	}
}

func TestParseExecRunConfigRequiresDescription(t *testing.T) {
	_, err := parseExecRunConfig(map[string]any{
		"command": "go version",
	})
	if err == nil {
		t.Fatal("expected missing description error")
	}
	if !strings.Contains(err.Error(), "description is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateWorkingDirectoryRejectsMissing(t *testing.T) {
	_, err := validateWorkingDirectory(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected missing directory error")
	}
}

func TestValidateWorkingDirectoryRejectsFile(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "x.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := validateWorkingDirectory(f)
	if err == nil {
		t.Fatal("expected file path rejection")
	}
}

func TestCommandSafetyRejectsInteractive(t *testing.T) {
	if err := validateCommandSafety("Read-Host 'x'"); err == nil {
		t.Fatal("expected interactive command rejection")
	}
	if err := validateCommandSafety("git rebase -i HEAD~1"); err == nil {
		t.Fatal("expected interactive git rejection")
	}
}

func TestCommandSafetyRejectsDestructive(t *testing.T) {
	if err := validateCommandSafety("rm -rf /"); err == nil {
		t.Fatal("expected dangerous rm rejection")
	}
	if err := validateCommandSafety("git reset --hard && git clean -fd"); err == nil {
		t.Fatal("expected destructive git rejection")
	}
}

func TestCommandSafetyAllowsNormalCommand(t *testing.T) {
	if err := validateCommandSafety("go version"); err != nil {
		t.Fatalf("expected command to be allowed, got %v", err)
	}
}

func TestFormatExecErrorNotFound(t *testing.T) {
	err := &exec.Error{Name: "no-such-binary", Err: exec.ErrNotFound}
	formatted := formatExecError(err)
	if !strings.Contains(strings.ToLower(formatted.Error()), "not found") {
		t.Fatalf("unexpected formatted error: %s", formatted)
	}
}

func TestExecRunReturnsStructuredOutput(t *testing.T) {
	e := &execTool{}
	res, err := e.run(context.Background(), map[string]any{
		"command":     "go version",
		"description": "Check Go version",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("expected empty tool error, got: %s", res.Error)
	}
	out := parseExecOutput(t, res.Output)
	if out.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", out.ExitCode)
	}
	if strings.TrimSpace(out.Stdout) == "" && strings.TrimSpace(out.Stderr) == "" {
		t.Fatal("expected stdout/stderr content")
	}
	if out.DurationMs <= 0 {
		t.Fatalf("expected duration_ms > 0, got %d", out.DurationMs)
	}
	if out.Shell == "" {
		t.Fatal("expected shell to be set")
	}
	if out.WorkingDirectory == "" {
		t.Fatal("expected working_directory to be set")
	}
}

func TestExecRunTimeoutProducesStructuredFlag(t *testing.T) {
	e := &execTool{}
	command := "sleep 2"
	if runtime.GOOS == "windows" {
		command = "Start-Sleep -Seconds 2"
	}
	res, err := e.run(context.Background(), map[string]any{
		"command":     command,
		"description": "Verify timeout handling",
		"timeout_ms":  "50",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("expected no tool-level error, got: %s", res.Error)
	}
	out := parseExecOutput(t, res.Output)
	if !out.TimedOut {
		t.Fatal("expected timed_out=true")
	}
	if out.ExitCode != -1 {
		t.Fatalf("expected timeout exit code -1, got %d", out.ExitCode)
	}
}

func TestExecRunDangerousCommandReturnsToolError(t *testing.T) {
	e := &execTool{}
	res, err := e.run(context.Background(), map[string]any{
		"command":     "rm -rf /",
		"description": "Verify dangerous command rejection",
	})
	if err == nil {
		t.Fatal("expected tool error for dangerous command")
	}
	if res != nil {
		t.Fatal("expected nil result when validation fails")
	}
}

func TestTrimOutputReportsTruncation(t *testing.T) {
	data := []byte(strings.Repeat("a", 70*1024))
	trimmed, truncated := trimOutput(data)
	if !truncated {
		t.Fatal("expected truncation=true")
	}
	if len(trimmed) == 0 || len(trimmed) >= len(data) {
		t.Fatalf("unexpected trimmed length: %d", len(trimmed))
	}
}

func TestEncodeExecOutputJSON(t *testing.T) {
	payload := execOutputPayload{
		Stdout:           "ok",
		Stderr:           "",
		ExitCode:         0,
		TimedOut:         false,
		Truncated:        false,
		Shell:            "bash",
		WorkingDirectory: "/tmp",
		DurationMs:       12,
	}
	encoded, err := encodeExecOutput(payload)
	if err != nil {
		t.Fatalf("encode output: %v", err)
	}
	parsed := parseExecOutput(t, encoded)
	if parsed.Stdout != "ok" || parsed.Shell != "bash" || parsed.DurationMs != 12 {
		t.Fatalf("unexpected parsed payload: %+v", parsed)
	}
}

func TestBuildExecInvocationBashFallbackToSh(t *testing.T) {
	orig := execLookPath
	t.Cleanup(func() { execLookPath = orig })
	execLookPath = func(file string) (string, error) {
		if file == "bash" {
			return "", errors.New("not found")
		}
		if file == "sh" {
			return "/bin/sh", nil
		}
		return "", errors.New("not found")
	}
	inv, err := buildExecInvocation("linux", execRunConfig{command: "echo ok", shell: "auto"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.commandName != "sh" {
		t.Fatalf("expected sh fallback, got %s", inv.commandName)
	}
}

func TestResolveWorkingDirectoryUsesCurrentWhenEmpty(t *testing.T) {
	cwd, err := resolveWorkingDirectory("")
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	if cwd == "" {
		t.Fatal("expected non-empty cwd")
	}
}

func TestExecRunUsesProvidedWorkingDirectory(t *testing.T) {
	e := &execTool{}
	dir := t.TempDir()
	res, err := e.run(context.Background(), map[string]any{
		"command":           "go version",
		"description":       "Verify custom working directory",
		"working_directory": dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected tool-level error: %s", res.Error)
	}
	out := parseExecOutput(t, res.Output)
	if out.WorkingDirectory != dir {
		t.Fatalf("expected working directory %s, got %s", dir, out.WorkingDirectory)
	}
}

func parseExecOutput(t *testing.T, raw string) execOutputPayload {
	t.Helper()
	var out execOutputPayload
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("invalid exec output json: %v\nraw: %s", err, raw)
	}
	return out
}

func TestResolveTimeoutMsInvalidFallsBackDefault(t *testing.T) {
	if got := resolveTimeoutMs("abc"); got != 30000 {
		t.Fatalf("expected default timeout for invalid input, got %d", got)
	}
}

func TestRunDurationIsMeasured(t *testing.T) {
	e := &execTool{}
	start := time.Now()
	res, err := e.run(context.Background(), map[string]any{
		"command":     "go version",
		"description": "Measure command duration",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := parseExecOutput(t, res.Output)
	if out.DurationMs < 0 {
		t.Fatalf("duration should be >= 0, got %d", out.DurationMs)
	}
	if time.Since(start).Milliseconds()+100 < out.DurationMs {
		t.Fatalf("duration_ms seems unrealistic: %d", out.DurationMs)
	}
}
