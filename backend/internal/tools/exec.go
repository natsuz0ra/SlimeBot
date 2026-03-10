package tools

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	execDefaultTimeout = 30
	execMaxTimeout     = 300
	execMaxOutputBytes = 64 * 1024 // 64KB
)

type execTool struct{}

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
				{Name: "command", Required: true, Description: "要执行的命令字符串", Example: "ls -la"},
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
	cmdStr := strings.TrimSpace(params["command"])
	if cmdStr == "" {
		return nil, fmt.Errorf("参数 command 不能为空")
	}

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

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}

	output, err := cmd.CombinedOutput()

	if len(output) > execMaxOutputBytes {
		output = output[:execMaxOutputBytes]
	}
	outStr := string(output)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &ExecuteResult{
				Output: outStr,
				Error:  fmt.Sprintf("命令执行超时（%d秒）", timeout),
			}, nil
		}
		return &ExecuteResult{
			Output: outStr,
			Error:  err.Error(),
		}, nil
	}

	return &ExecuteResult{Output: outStr}, nil
}
