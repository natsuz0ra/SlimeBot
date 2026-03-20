package tools

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"slimebot/backend/internal/constants"
)

func TestBuildExecInvocationProgramArgs(t *testing.T) {
	inv, err := buildExecInvocation(runtime.GOOS, "go", `["version"]`, "", "none")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if inv.commandName != "go" {
		t.Fatalf("unexpected command name: %s", inv.commandName)
	}
	if len(inv.commandArgs) != 1 || inv.commandArgs[0] != "version" {
		t.Fatalf("unexpected command args: %#v", inv.commandArgs)
	}
}

func TestBuildExecInvocationInvalidArgsJSON(t *testing.T) {
	_, err := buildExecInvocation(runtime.GOOS, "go", `["version"`, "", "none")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse args") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildExecInvocationArgsWithoutProgram(t *testing.T) {
	_, err := buildExecInvocation(runtime.GOOS, "", `["version"]`, "echo hello", "none")
	if err == nil {
		t.Fatal("expected params error, got nil")
	}
	if !strings.Contains(err.Error(), "args can only be used with program") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildExecInvocationWindowsShellSelection(t *testing.T) {
	defaultInv, err := buildExecInvocation("windows", "", "", "echo hello", "none")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if defaultInv.commandName != "powershell" {
		t.Fatalf("expected powershell by default on windows, got %s", defaultInv.commandName)
	}

	cmdInv, err := buildExecInvocation("windows", "", "", "echo hello", "cmd")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if cmdInv.commandName != "cmd" {
		t.Fatalf("expected cmd shell, got %s", cmdInv.commandName)
	}
}

func TestTrimOutput(t *testing.T) {
	large := strings.Repeat("a", constants.ExecMaxOutputBytes+100)
	trimmed := trimOutput([]byte(large))
	if len(trimmed) != constants.ExecMaxOutputBytes {
		t.Fatalf("unexpected trimmed size: %d", len(trimmed))
	}
}

func TestDecodeCommandOutputUTF8(t *testing.T) {
	raw := []byte("中文输出")
	decoded := decodeCommandOutput("windows", raw)
	if decoded != "中文输出" {
		t.Fatalf("unexpected decoded output: %s", decoded)
	}
}

func TestDecodeCommandOutputUTF16LEBOM(t *testing.T) {
	raw := []byte{
		0xFF, 0xFE,
		0x2D, 0x4E, // 中
		0x87, 0x65, // 文
		0x93, 0x8F, // 输
		0xFA, 0x51, // 出
	}
	decoded := decodeCommandOutput("windows", raw)
	if decoded != "中文输出" {
		t.Fatalf("unexpected utf16 decoded output: %s", decoded)
	}
}

func TestDecodeCommandOutputGB18030OnWindows(t *testing.T) {
	raw, err := encodeGB18030("中文预览")
	if err != nil {
		t.Fatalf("encode gb18030 failed: %v", err)
	}
	decoded := decodeCommandOutput("windows", raw)
	if decoded != "中文预览" {
		t.Fatalf("unexpected gb18030 decoded output: %s", decoded)
	}
}

func TestDecodeCommandOutputGB18030NoFallbackOnLinux(t *testing.T) {
	raw, err := encodeGB18030("中文预览")
	if err != nil {
		t.Fatalf("encode gb18030 failed: %v", err)
	}
	decoded := decodeCommandOutput("linux", raw)
	if decoded == "中文预览" {
		t.Fatal("expected linux path to avoid gb18030 fallback decoding")
	}
}

func TestDecodeCommandOutputGB18030WithTruncatedTail(t *testing.T) {
	raw, err := encodeGB18030(strings.Repeat("中", 30000))
	if err != nil {
		t.Fatalf("encode gb18030 failed: %v", err)
	}
	trimmed := trimOutput(raw)
	decoded := decodeCommandOutput("windows", trimmed)
	if strings.Contains(decoded, "\uFFFD") {
		preview := decoded
		if len(preview) > 50 {
			preview = preview[:50]
		}
		t.Fatalf("decoded output should not contain replacement chars: %q", preview)
	}
	if decoded == "" {
		t.Fatal("decoded output should not be empty")
	}
}

func TestBuildShellInvocationWindowsPowerShellForcesUTF8(t *testing.T) {
	inv := buildShellInvocation("windows", "Write-Output '中文'", "powershell")
	if inv.commandName != "powershell" {
		t.Fatalf("expected powershell command, got %s", inv.commandName)
	}
	if len(inv.commandArgs) < 4 {
		t.Fatalf("unexpected command args length: %#v", inv.commandArgs)
	}
	if inv.commandArgs[2] != "-Command" {
		t.Fatalf("expected -Command argument, got %#v", inv.commandArgs)
	}
	if !strings.Contains(inv.commandArgs[3], "OutputEncoding") {
		t.Fatalf("expected utf-8 output encoding setup in command, got %s", inv.commandArgs[3])
	}
}

func TestFormatExecErrorNotFound(t *testing.T) {
	err := &exec.Error{Name: "no-such-binary", Err: exec.ErrNotFound}
	formatted := formatExecError(err)
	if !strings.Contains(formatted, "Command not found") {
		t.Fatalf("unexpected formatted error: %s", formatted)
	}
}

func TestExecRunProgramArgsSuccess(t *testing.T) {
	e := &execTool{}
	result, err := e.run(map[string]string{
		"program": "go",
		"args":    `["version"]`,
		"timeout": "10",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Error != "" {
		t.Fatalf("expected empty result error, got %s", result.Error)
	}
	if !strings.Contains(strings.ToLower(result.Output), "go version") {
		t.Fatalf("expected go version output, got %s", result.Output)
	}
}

func TestExecRunTimeout(t *testing.T) {
	e := &execTool{}
	command := "sleep 2"
	if runtime.GOOS == "windows" {
		command = "Start-Sleep -Seconds 2"
	}
	result, err := e.run(map[string]string{
		"command": command,
		"timeout": "1",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !strings.Contains(result.Error, "Command timed out") {
		t.Fatalf("expected timeout error, got %s", result.Error)
	}
}

func encodeGB18030(input string) ([]byte, error) {
	encoded, _, err := transform.String(simplifiedchinese.GB18030.NewEncoder(), input)
	if err != nil {
		return nil, err
	}
	return []byte(encoded), nil
}
