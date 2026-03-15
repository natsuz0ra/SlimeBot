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
)

const (
	execDefaultTimeout = 30
	execMaxTimeout     = 300
	execMaxOutputBytes = 64 * 1024 // 64KB
)

type execTool struct{}

type execInvocation struct {
	commandName string
	commandArgs []string
}

func init() {
	Register(&execTool{})
}

func (e *execTool) Name() string { return "exec" }

func (e *execTool) Description() string {
	return "命令行工具，可以在运行服务的设备上执行系统命令并返回输出结果"
}

func (e *execTool) Commands() []Command {
	return []Command{
		{
			Name:        "run",
			Description: "执行一条系统命令，返回标准输出和标准错误的合并结果",
			Params: []CommandParam{
				{Name: "program", Required: false, Description: "可执行程序名（推荐，优先于 command）", Example: "python"},
				{Name: "args", Required: false, Description: "程序参数（JSON 字符串数组）", Example: "[\"-c\",\"print('hello')\"]"},
				{Name: "shell", Required: false, Description: "command 模式下使用的 shell：none|powershell|cmd（Windows 默认 none=PowerShell）", Example: "none"},
				{Name: "command", Required: false, Description: "要执行的命令字符串（兼容模式）", Example: "echo hello"},
				{Name: "timeout", Required: false, Description: "命令执行超时秒数，默认30秒，最大300秒", Example: "60"},
			},
		},
	}
}

func (e *execTool) Execute(command string, params map[string]string) (*ExecuteResult, error) {
	switch command {
	case "run":
		return e.run(params)
	default:
		return nil, fmt.Errorf("exec 工具不支持命令: %s", command)
	}
}

func (e *execTool) run(params map[string]string) (*ExecuteResult, error) {
	program := strings.TrimSpace(params["program"])
	cmdStr := strings.TrimSpace(params["command"])
	argsRaw := strings.TrimSpace(params["args"])
	shell := normalizeShell(params["shell"])

	timeout := execDefaultTimeout
	if ts := strings.TrimSpace(params["timeout"]); ts != "" {
		if v, err := strconv.Atoi(ts); err == nil && v > 0 {
			timeout = v
		}
	}
	if timeout > execMaxTimeout {
		timeout = execMaxTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
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
				Error:  fmt.Sprintf("命令执行超时（%d秒）", timeout),
			}, nil
		}
		return &ExecuteResult{
			Output: outStr,
			Error:  formatExecError(err),
		}, nil
	}

	return &ExecuteResult{Output: outStr}, nil
}

func buildExecInvocation(goos, program, argsRaw, command, shell string) (execInvocation, error) {
	if shell != "" && shell != "none" && shell != "powershell" && shell != "cmd" {
		return execInvocation{}, fmt.Errorf("参数 shell 非法：%s（可选值: none|powershell|cmd）", shell)
	}

	if program != "" {
		if command != "" {
			return execInvocation{}, fmt.Errorf("参数 program 与 command 不能同时传入，请二选一")
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
		return execInvocation{}, fmt.Errorf("参数 args 仅可与 program 一起使用")
	}
	if command == "" {
		return execInvocation{}, fmt.Errorf("参数 command 不能为空（或改用 program + args）")
	}

	return buildShellInvocation(goos, command, shell), nil
}

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

func parseJSONArgs(argsRaw string) ([]string, error) {
	if strings.TrimSpace(argsRaw) == "" {
		return nil, nil
	}
	var parsed []string
	if err := json.Unmarshal([]byte(argsRaw), &parsed); err != nil {
		return nil, fmt.Errorf("参数 args 解析失败，需为 JSON 字符串数组: %w", err)
	}
	return parsed, nil
}

func trimOutput(output []byte) []byte {
	if len(output) > execMaxOutputBytes {
		return output[:execMaxOutputBytes]
	}
	return output
}

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

func formatExecError(err error) string {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return fmt.Sprintf("命令不存在: %s", execErr.Name)
	}
	return err.Error()
}
