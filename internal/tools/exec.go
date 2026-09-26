package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"slimebot/internal/constants"
	sandboxpolicy "slimebot/internal/sandbox"
)

const (
	defaultTimeoutMs                  = constants.ExecDefaultTimeoutMs
	maxTimeoutMs                      = constants.ExecMaxTimeoutMs
	execSandboxPermissionsDefault     = "default"
	execSandboxPermissionsReqApproval = "required_approval"
)

var execLookPath = exec.LookPath

// execTool runs host commands.
type execTool struct{}

// execRunConfig is the parsed configuration for exec__run.
type execRunConfig struct {
	command            string
	shell              string
	timeoutMs          int
	workingDirectory   string
	description        string
	sandboxPermissions string
	background         bool
}

// execInvocation is a single os/exec invocation.
type execInvocation struct {
	commandName      string
	commandArgs      []string
	shell            string
	workingDirectory string
}

type execOutputPayload struct {
	Stdout             string `json:"stdout"`
	Stderr             string `json:"stderr"`
	ExitCode           int    `json:"exit_code"`
	TimedOut           bool   `json:"timed_out"`
	Truncated          bool   `json:"truncated"`
	Shell              string `json:"shell"`
	WorkingDirectory   string `json:"working_directory"`
	DurationMs         int64  `json:"duration_ms"`
	SandboxPermissions string `json:"sandbox_permissions"`
	SandboxMode        string `json:"sandbox_mode"`
	Background         bool   `json:"background,omitempty"`
	ProcessID          string `json:"process_id,omitempty"`
}

func init() {
	Register(&execTool{})
}

func (e *execTool) Name() string { return "exec" }

func (e *execTool) Description() string {
	return "Execute one terminal command using shell auto-routing (Windows=PowerShell, Linux/macOS=bash/sh). Use for host command execution only, including from subagents under the parent approval flow. Prefer specialized tools for file reads/writes/search. Avoid interactive commands and dangerous destructive operations unless explicitly requested and approved."
}

func (e *execTool) Commands() []Command {
	return []Command{
		{
			Name:        "run",
			Description: "Run exactly one command and return structured JSON output: stdout/stderr/exit_code/timed_out/truncated/shell/working_directory/duration_ms/sandbox_permissions. Subagent calls inherit the parent approval mode and must include a bounded description or reason. Keep command concise, quote paths with spaces, avoid unnecessary sleep/poll loops, and use safer git operations by default.",
			Params: []CommandParam{
				{Name: "command", Required: true, Description: "Single command string to execute.", Example: "go test ./..."},
				{Name: "timeout_ms", Required: false, Description: "Optional timeout in milliseconds. Default 30000, max 600000.", Example: "120000"},
				{Name: "shell", Required: false, Description: "Shell selection: auto|bash|sh|powershell|cmd. Default auto.", Example: "auto"},
				{Name: "working_directory", Required: false, Description: "Optional working directory. Must exist and be a directory.", Example: "g:\\gitCode\\SlimeBot"},
				{Name: "description", Required: false, Description: "Short human-readable intent for approval and audit. Required unless reason is provided.", Example: "Run unit tests for tools package"},
				{Name: "reason", Required: false, Description: "Alternative short audit reason when description is not provided.", Example: "Verify the subagent's code change"},
				{Name: "sandbox_permissions", Required: false, Description: "Permission intent for approval context: default or required_approval. This does not bypass approval; subagents inherit the parent approval flow.", Example: "required_approval", Schema: map[string]any{"type": "string", "enum": []string{execSandboxPermissionsDefault, execSandboxPermissionsReqApproval}}},
				{Name: "background", Required: false, Description: "Start the command as a managed background process and return process_id immediately.", Example: "false"},
			},
		},
	}
}

func (e *execTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "run":
		return e.run(ctx, params)
	default:
		return nil, fmt.Errorf("exec tool does not support command: %s", command)
	}
}

// run parses params, executes command, and returns structured JSON output.
func (e *execTool) run(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	cfg, err := parseExecRunConfig(params, ctx)
	if err != nil {
		return nil, err
	}
	if err := validateCommandSafety(cfg.command); err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	invocation, err := buildExecInvocation(runtime.GOOS, cfg)
	if err != nil {
		return nil, err
	}
	if cfg.sandboxPermissions == execSandboxPermissionsReqApproval && !sandboxpolicy.HasEscalationGrant(ctx) {
		return nil, fmt.Errorf("sandbox escalation requires approval before execution")
	}
	policy, _ := sandboxpolicy.FromContext(ctx)
	sandboxMode := string(sandboxpolicy.ModeDangerFullAccess)
	if policy != nil {
		sandboxMode = string(policy.Mode())
	}
	if cfg.background {
		manager := processManagerFromContext(ctx)
		processID := manager.Start(cfg.command, func(runCtx context.Context) processRunResult {
			start := time.Now()
			result, runErr := sandboxpolicy.RunCommand(runCtx, policy, sandboxpolicy.CommandRequest{
				CommandName: invocation.commandName,
				CommandArgs: invocation.commandArgs,
				Dir:         invocation.workingDirectory,
				Timeout:     time.Duration(cfg.timeoutMs) * time.Millisecond,
			})
			stdoutTrimmed, _ := trimOutput(result.Stdout)
			stderrTrimmed, _ := trimOutput(result.Stderr)
			runResult := processRunResult{
				Stdout:     decodeCommandOutput(runtime.GOOS, stdoutTrimmed),
				Stderr:     decodeCommandOutput(runtime.GOOS, stderrTrimmed),
				ExitCode:   result.ExitCode,
				TimedOut:   result.TimedOut,
				DurationMs: result.DurationMs,
			}
			if runResult.DurationMs == 0 {
				runResult.DurationMs = time.Since(start).Milliseconds()
			}
			if runErr != nil {
				runResult.Error = formatExecError(runErr).Error()
			}
			return runResult
		})
		out, err := encodeExecOutput(execOutputPayload{
			Stdout:             "",
			Stderr:             "",
			ExitCode:           0,
			TimedOut:           false,
			Truncated:          false,
			Shell:              invocation.shell,
			WorkingDirectory:   invocation.workingDirectory,
			DurationMs:         0,
			SandboxPermissions: cfg.sandboxPermissions,
			SandboxMode:        sandboxMode,
			Background:         true,
			ProcessID:          processID,
		})
		if err != nil {
			return nil, err
		}
		return &ExecuteResult{Output: out}, nil
	}
	result, err := sandboxpolicy.RunCommand(ctx, policy, sandboxpolicy.CommandRequest{
		CommandName: invocation.commandName,
		CommandArgs: invocation.commandArgs,
		Dir:         invocation.workingDirectory,
		Timeout:     time.Duration(cfg.timeoutMs) * time.Millisecond,
	})

	stdoutTrimmed, stdoutTruncated := trimOutput(result.Stdout)
	stderrTrimmed, stderrTruncated := trimOutput(result.Stderr)
	payload := execOutputPayload{
		Stdout:             decodeCommandOutput(runtime.GOOS, stdoutTrimmed),
		Stderr:             decodeCommandOutput(runtime.GOOS, stderrTrimmed),
		ExitCode:           result.ExitCode,
		TimedOut:           result.TimedOut,
		Truncated:          stdoutTruncated || stderrTruncated,
		Shell:              invocation.shell,
		WorkingDirectory:   invocation.workingDirectory,
		DurationMs:         result.DurationMs,
		SandboxPermissions: cfg.sandboxPermissions,
		SandboxMode:        sandboxMode,
	}

	if err != nil {
		return nil, formatExecError(err)
	}

	out, err := encodeExecOutput(payload)
	if err != nil {
		return nil, err
	}
	_ = cfg.description
	return &ExecuteResult{Output: out}, nil
}

func parseExecRunConfig(params map[string]any, contexts ...context.Context) (execRunConfig, error) {
	cfg := execRunConfig{}
	cfg.command = paramStringTrim(params, "command")
	cfg.description = paramStringTrim(params, "description")
	if cfg.description == "" {
		cfg.description = paramStringTrim(params, "reason")
	}
	cfg.shell = normalizeShell(paramString(params, "shell"))
	if cfg.shell == "" {
		cfg.shell = "auto"
	}
	cfg.sandboxPermissions = normalizeSandboxPermissions(paramString(params, "sandbox_permissions"))
	if cfg.sandboxPermissions == "" {
		cfg.sandboxPermissions = execSandboxPermissionsDefault
	}
	if cfg.sandboxPermissions != execSandboxPermissionsDefault && cfg.sandboxPermissions != execSandboxPermissionsReqApproval {
		return execRunConfig{}, fmt.Errorf("invalid sandbox_permissions value: %s (allowed: default|required_approval)", cfg.sandboxPermissions)
	}
	cfg.timeoutMs = resolveTimeoutMs(paramString(params, "timeout_ms"))
	background, _, err := paramBool(params, "background")
	if err != nil {
		return execRunConfig{}, err
	}
	cfg.background = background

	wd, err := resolveWorkingDirectory(paramString(params, "working_directory"), contexts...)
	if err != nil {
		return execRunConfig{}, err
	}
	cfg.workingDirectory = wd
	if cfg.command == "" {
		return execRunConfig{}, fmt.Errorf("command is required")
	}
	if cfg.description == "" {
		return execRunConfig{}, fmt.Errorf("description or reason is required")
	}
	return cfg, nil
}

func normalizeSandboxPermissions(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func resolveTimeoutMs(raw string) int {
	timeout := defaultTimeoutMs
	if ts := strings.TrimSpace(raw); ts != "" {
		if v, err := strconv.Atoi(ts); err == nil && v > 0 {
			timeout = v
		}
	}
	if timeout > maxTimeoutMs {
		return maxTimeoutMs
	}
	return timeout
}

func resolveWorkingDirectory(raw string, contexts ...context.Context) (string, error) {
	candidate := strings.TrimSpace(raw)
	if len(contexts) > 0 {
		if directory := sandboxpolicy.WorkingDirectoryFromContext(contexts[0]); directory != "" {
			if candidate == "" {
				candidate = directory
			} else if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(directory, candidate)
			}
		}
	}
	if candidate == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to resolve current working directory: %w", err)
		}
		candidate = cwd
	}
	return validateWorkingDirectory(candidate)
}

func validateWorkingDirectory(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("working_directory is invalid: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working_directory must be a directory")
	}
	resolved, err := filepathAbs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve working_directory: %w", err)
	}
	return resolved, nil
}

var filepathAbs = func(path string) (string, error) {
	return filepath.Abs(path)
}

func buildExecInvocation(goos string, cfg execRunConfig) (execInvocation, error) {
	if cfg.command == "" {
		return execInvocation{}, fmt.Errorf("command is required")
	}
	if cfg.shell != "auto" && cfg.shell != "bash" && cfg.shell != "sh" && cfg.shell != "powershell" && cfg.shell != "cmd" {
		return execInvocation{}, fmt.Errorf("invalid shell value: %s (allowed: auto|bash|sh|powershell|cmd)", cfg.shell)
	}

	shell := cfg.shell
	if shell == "auto" {
		if goos == "windows" {
			shell = "powershell"
		} else {
			shell = resolvePosixAutoShell()
		}
	}

	switch shell {
	case "powershell":
		return execInvocation{
			commandName:      "powershell",
			commandArgs:      []string{"-NoProfile", "-NonInteractive", "-Command", wrapPowerShellUTF8Command(cfg.command)},
			shell:            "powershell",
			workingDirectory: cfg.workingDirectory,
		}, nil
	case "cmd":
		return execInvocation{commandName: "cmd", commandArgs: []string{"/C", cfg.command}, shell: "cmd", workingDirectory: cfg.workingDirectory}, nil
	case "bash":
		return execInvocation{commandName: "bash", commandArgs: []string{"-lc", cfg.command}, shell: "bash", workingDirectory: cfg.workingDirectory}, nil
	case "sh":
		return execInvocation{commandName: "sh", commandArgs: []string{"-c", cfg.command}, shell: "sh", workingDirectory: cfg.workingDirectory}, nil
	default:
		return execInvocation{}, fmt.Errorf("unsupported shell: %s", shell)
	}
}

func resolvePosixAutoShell() string {
	if _, err := execLookPath("bash"); err == nil {
		return "bash"
	}
	return "sh"
}

func validateCommandSafety(command string) error {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return fmt.Errorf("command is required")
	}
	lower := strings.ToLower(trimmed)

	interactivePatterns := []string{
		"read-host",
		" pause",
		"pause ",
		" git add -i",
		" git rebase -i",
		"git add -i",
		"git rebase -i",
	}
	for _, p := range interactivePatterns {
		if strings.Contains(lower, p) {
			return fmt.Errorf("command rejected: interactive/blocking command is not allowed in exec tool")
		}
	}

	dangerPatterns := []string{
		"rm -rf /",
		"rm -fr /",
		"rm -rf --no-preserve-root /",
		"git reset --hard && git clean -fd",
		"git reset --hard && git clean -xdf",
		"git clean -fd && git reset --hard",
		"git clean -xdf && git reset --hard",
	}
	for _, p := range dangerPatterns {
		if strings.Contains(lower, p) {
			return fmt.Errorf("command rejected: dangerous destructive pattern detected")
		}
	}

	return nil
}

func encodeExecOutput(payload execOutputPayload) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to encode exec output: %w", err)
	}
	return string(b), nil
}

// trimOutput caps output size to avoid huge context payloads.
func trimOutput(output []byte) ([]byte, bool) {
	if len(output) > constants.ExecMaxOutputBytes {
		return output[:constants.ExecMaxOutputBytes], true
	}
	return output, false
}

// decodeCommandOutput decodes process output using BOM/heuristics for readability.
func decodeCommandOutput(goos string, output []byte) string {
	if len(output) == 0 {
		return ""
	}

	if bytes.HasPrefix(output, []byte{0xEF, 0xBB, 0xBF}) {
		return string(output[3:])
	}
	if bytes.HasPrefix(output, []byte{0xFF, 0xFE}) {
		if decoded, ok := decodeUTF16(output[2:], true); ok {
			return decoded
		}
	}
	if bytes.HasPrefix(output, []byte{0xFE, 0xFF}) {
		if decoded, ok := decodeUTF16(output[2:], false); ok {
			return decoded
		}
	}

	if isValidUTF8(output) {
		return string(output)
	}

	if goos == "windows" {
		if decoded, ok := decodeGB18030(output); ok {
			return decoded
		}
	}

	return string(output)
}

// decodeUTF16 decodes UTF-16 byte streams to a UTF-8 string.
func decodeUTF16(input []byte, littleEndian bool) (string, bool) {
	if len(input) == 0 {
		return "", true
	}
	if len(input)%2 != 0 {
		input = input[:len(input)-1]
	}
	if len(input) == 0 {
		return "", true
	}

	endianness := unicode.BigEndian
	if littleEndian {
		endianness = unicode.LittleEndian
	}
	decoded, _, err := transform.String(unicode.UTF16(endianness, unicode.IgnoreBOM).NewDecoder(), string(input))
	if err != nil {
		return "", false
	}
	return decoded, true
}

// decodeGB18030 tries GB18030 on Windows for common legacy console encodings.
func decodeGB18030(input []byte) (string, bool) {
	for cut := 0; cut < 4; cut++ {
		if len(input)-cut <= 0 {
			break
		}
		decoded, _, err := transform.String(simplifiedchinese.GB18030.NewDecoder(), string(input[:len(input)-cut]))
		if err == nil {
			return decoded, true
		}
	}
	return "", false
}

// wrapPowerShellUTF8Command forces PowerShell stdin/stdout to UTF-8.
func wrapPowerShellUTF8Command(command string) string {
	prefix := "[Console]::InputEncoding=[System.Text.Encoding]::UTF8; [Console]::OutputEncoding=[System.Text.Encoding]::UTF8; $OutputEncoding=[System.Text.Encoding]::UTF8"
	if strings.TrimSpace(command) == "" {
		return prefix
	}
	return prefix + "; " + command
}

func isValidUTF8(input []byte) bool {
	return utf8.Valid(input)
}

func normalizeShell(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// formatExecError normalizes low-level exec errors for clearer messages.
func formatExecError(err error) error {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return fmt.Errorf("command not found: %s", execErr.Name)
	}
	return err
}
