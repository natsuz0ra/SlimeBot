package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"slimebot/internal/constants"
)

// execTool 提供本机命令执行能力。
type execTool struct{}

// execInvocation 描述一次可直接交给 os/exec 的调用。
type execInvocation struct {
	commandName string
	commandArgs []string
}

func init() {
	Register(&execTool{})
}

func (e *execTool) Name() string { return "exec" }

func (e *execTool) Description() string {
	return "Run system commands on the host and return command output."
}

func (e *execTool) Commands() []Command {
	return []Command{
		{
			Name:        "run",
			Description: "Run a system command and return combined stdout/stderr output.",
			Params: []CommandParam{
				{Name: "program", Required: false, Description: "Executable name (recommended; takes precedence over command).", Example: "python"},
				{Name: "args", Required: false, Description: "Program arguments (JSON string array).", Example: "[\"-c\",\"print('hello')\"]"},
				{Name: "shell", Required: false, Description: "Shell used in command mode: none|powershell|cmd (Windows default none=PowerShell).", Example: "none"},
				{Name: "command", Required: false, Description: "Command string to execute (compatibility mode).", Example: "echo hello"},
				{Name: "timeout", Required: false, Description: "Command timeout in seconds, default 30, max 300.", Example: "60"},
			},
		},
	}
}

func (e *execTool) Execute(ctx context.Context, command string, params map[string]string) (*ExecuteResult, error) {
	switch command {
	case "run":
		return e.run(ctx, params)
	default:
		return nil, fmt.Errorf("exec tool does not support command: %s", command)
	}
}

// run 解析入参并执行命令，统一返回输出与错误信息。
func (e *execTool) run(ctx context.Context, params map[string]string) (*ExecuteResult, error) {
	program := strings.TrimSpace(params["program"])
	cmdStr := strings.TrimSpace(params["command"])
	argsRaw := strings.TrimSpace(params["args"])
	shell := normalizeShell(params["shell"])

	timeout := constants.ExecDefaultTimeout
	if ts := strings.TrimSpace(params["timeout"]); ts != "" {
		if v, err := strconv.Atoi(ts); err == nil && v > 0 {
			timeout = v
		}
	}
	if timeout > constants.ExecMaxTimeout {
		timeout = constants.ExecMaxTimeout
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	invocation, parseErr := buildExecInvocation(runtime.GOOS, program, argsRaw, cmdStr, shell)
	if parseErr != nil {
		return nil, parseErr
	}
	cmd := exec.CommandContext(ctx, invocation.commandName, invocation.commandArgs...)

	output, err := cmd.CombinedOutput()
	outStr := decodeCommandOutput(runtime.GOOS, trimOutput(output))

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &ExecuteResult{
				Output: outStr,
				Error:  fmt.Sprintf("Command timed out (%d seconds).", timeout),
			}, nil
		}
		return &ExecuteResult{
			Output: outStr,
			Error:  formatExecError(err),
		}, nil
	}

	return &ExecuteResult{Output: outStr}, nil
}

// buildExecInvocation 统一解析 program/args 或 command/shell 两种调用模式。
func buildExecInvocation(goos, program, argsRaw, command, shell string) (execInvocation, error) {
	if shell != "" && shell != "none" && shell != "powershell" && shell != "cmd" {
		return execInvocation{}, fmt.Errorf("Invalid shell value: %s (allowed: none|powershell|cmd).", shell)
	}

	if program != "" {
		if command != "" {
			return execInvocation{}, fmt.Errorf("program and command cannot be provided together; choose one.")
		}
		parsedArgs, err := parseJSONArgs(argsRaw)
		if err != nil {
			return execInvocation{}, err
		}
		return execInvocation{
			commandName: program,
			commandArgs: parsedArgs,
		}, nil
	}

	if argsRaw != "" {
		return execInvocation{}, fmt.Errorf("args can only be used with program.")
	}
	if command == "" {
		return execInvocation{}, fmt.Errorf("command is required (or use program + args).")
	}

	return buildShellInvocation(goos, command, shell), nil
}

// buildShellInvocation 根据平台和 shell 选项构造最终可执行命令。
func buildShellInvocation(goos, command, shell string) execInvocation {
	normalizedShell := shell
	if normalizedShell == "" || normalizedShell == "none" {
		if goos == "windows" {
			normalizedShell = "powershell"
		} else {
			normalizedShell = "sh"
		}
	}
	switch normalizedShell {
	case "cmd":
		return execInvocation{commandName: "cmd", commandArgs: []string{"/C", command}}
	case "powershell":
		return execInvocation{
			commandName: "powershell",
			commandArgs: []string{"-NoProfile", "-NonInteractive", "-Command", wrapPowerShellUTF8Command(command)},
		}
	case "sh":
		return execInvocation{commandName: "sh", commandArgs: []string{"-c", command}}
	default:
		return execInvocation{commandName: "sh", commandArgs: []string{"-c", command}}
	}
}

// parseJSONArgs 解析 JSON 字符串数组形式的命令参数。
func parseJSONArgs(argsRaw string) ([]string, error) {
	if strings.TrimSpace(argsRaw) == "" {
		return nil, nil
	}
	var parsed []string
	if err := json.Unmarshal([]byte(argsRaw), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse args; expected a JSON string array: %w", err)
	}
	return parsed, nil
}

// trimOutput 限制输出字节数，避免超长结果撑爆上下文。
func trimOutput(output []byte) []byte {
	if len(output) > constants.ExecMaxOutputBytes {
		return output[:constants.ExecMaxOutputBytes]
	}
	return output
}

// decodeCommandOutput 按 BOM/编码策略解码进程输出，优先保证可读性。
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

// decodeUTF16 尝试把 UTF-16 字节流解码为 UTF-8 字符串。
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

// decodeGB18030 在 Windows 场景尝试兼容常见中文编码输出。
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

// wrapPowerShellUTF8Command 强制 PowerShell 输入输出编码为 UTF-8。
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

// formatExecError 规范化底层执行错误，提升错误信息可读性。
func formatExecError(err error) string {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return fmt.Sprintf("Command not found: %s.", execErr.Name)
	}
	return err.Error()
}
