package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type CommandRequest struct {
	CommandName    string
	CommandArgs    []string
	Dir            string
	Env            []string
	Timeout        time.Duration
	MaxOutputBytes int // Per stream; zero preserves the caller's unlimited capture.
}

type CommandResult struct {
	Stdout     []byte
	Stderr     []byte
	ExitCode   int
	TimedOut   bool
	DurationMs int64
	Truncated  bool
}

var lookPath = exec.LookPath
var commandContext = exec.CommandContext

func RunCommand(ctx context.Context, policy *Policy, req CommandRequest) (CommandResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if policy == nil || (policy.Mode() == ModeDangerFullAccess && policy.Network().Enabled) {
		return runHostCommand(ctx, req.CommandName, req.CommandArgs, req.Dir, req.Env, req.Timeout, req.MaxOutputBytes)
	}
	if policy.Mode() == ModeDangerFullAccess {
		if runtime.GOOS == "windows" {
			return CommandResult{}, fmt.Errorf("network-only sandbox execution is unsupported on windows")
		}
		commandName, commandArgs, err := buildNetworkOnlySandboxedCommand(req)
		if err != nil {
			return CommandResult{}, err
		}
		return runHostCommand(ctx, commandName, commandArgs, req.Dir, req.Env, req.Timeout, req.MaxOutputBytes)
	}
	if err := policy.CheckRead(req.Dir); err != nil {
		return CommandResult{}, err
	}
	if runtime.GOOS == "windows" {
		return CommandResult{}, fmt.Errorf("sandbox execution is unsupported on windows; use %s with explicit approval to run without sandbox", ModeDangerFullAccess)
	}
	commandName, commandArgs, err := buildSandboxedCommand(policy, req)
	if err != nil {
		return CommandResult{}, err
	}
	return runHostCommand(ctx, commandName, commandArgs, req.Dir, req.Env, req.Timeout, req.MaxOutputBytes)
}

type cappedOutput struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedOutput) Len() int       { return b.buffer.Len() }
func (b *cappedOutput) Bytes() []byte  { return b.buffer.Bytes() }
func (b *cappedOutput) String() string { return b.buffer.String() }

func (b *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	keep := n
	if b.limit > 0 {
		keep = min(keep, max(0, b.limit-b.Len()))
	}
	_, _ = b.buffer.Write(p[:keep])
	if keep < n {
		b.truncated = true
	}
	return n, nil
}

func runHostCommand(ctx context.Context, commandName string, commandArgs []string, dir string, env []string, timeout time.Duration, limits ...int) (CommandResult, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := commandContext(ctx, commandName, commandArgs...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = env
	}
	limit := 0
	if len(limits) > 0 {
		limit = limits[0]
	}
	stdout := cappedOutput{limit: limit}
	stderr := cappedOutput{limit: limit}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	err := cmd.Run()
	result := CommandResult{
		Stdout:     stdout.Bytes(),
		Stderr:     stderr.Bytes(),
		ExitCode:   0,
		TimedOut:   ctx.Err() == context.DeadlineExceeded,
		DurationMs: time.Since(start).Milliseconds(),
		Truncated:  stdout.truncated || stderr.truncated,
	}
	if result.TimedOut {
		result.ExitCode = -1
		return result, nil
	}
	if err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, err
	}
	return result, nil
}

func asExitError(err error, target **exec.ExitError) bool {
	return errors.As(err, target)
}

func buildSandboxedCommand(policy *Policy, req CommandRequest) (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		if _, err := lookPath("/usr/bin/sandbox-exec"); err != nil {
			return "", nil, fmt.Errorf("sandbox execution unavailable: /usr/bin/sandbox-exec not found")
		}
		args := []string{"-p", buildSeatbeltProfile(policy), req.CommandName}
		args = append(args, req.CommandArgs...)
		return "/usr/bin/sandbox-exec", args, nil
	case "linux":
		bwrap, err := lookPath("bwrap")
		if err != nil {
			return "", nil, fmt.Errorf("sandbox execution unavailable: bubblewrap (bwrap) not found")
		}
		args := buildBubblewrapArgs(policy, req)
		return bwrap, args, nil
	default:
		return "", nil, fmt.Errorf("sandbox execution unsupported on %s", runtime.GOOS)
	}
}

func buildNetworkOnlySandboxedCommand(req CommandRequest) (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		if _, err := lookPath("/usr/bin/sandbox-exec"); err != nil {
			return "", nil, fmt.Errorf("network-only sandbox execution unavailable: /usr/bin/sandbox-exec not found")
		}
		args := []string{"-p", buildSeatbeltNetworkOnlyProfile(), req.CommandName}
		args = append(args, req.CommandArgs...)
		return "/usr/bin/sandbox-exec", args, nil
	case "linux":
		bwrap, err := lookPath("bwrap")
		if err != nil {
			return "", nil, fmt.Errorf("network-only sandbox execution unavailable: bubblewrap (bwrap) not found")
		}
		return bwrap, buildBubblewrapNetworkOnlyArgs(req), nil
	default:
		return "", nil, fmt.Errorf("network-only sandbox execution unsupported on %s", runtime.GOOS)
	}
}

func buildSeatbeltProfile(policy *Policy) string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	b.WriteString("(allow process*)\n")
	b.WriteString("(allow sysctl*)\n")
	b.WriteString("(allow file-read*)\n")
	b.WriteString("(allow file-write* (literal \"/dev/null\"))\n")
	for _, root := range policy.WritableRoots() {
		b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", root))
	}
	if policy.Network().Enabled {
		b.WriteString("(allow network*)\n")
	}
	return b.String()
}

func buildSeatbeltNetworkOnlyProfile() string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(allow default)\n")
	b.WriteString("(deny network*)\n")
	return b.String()
}

func buildBubblewrapArgs(policy *Policy, req CommandRequest) []string {
	args := []string{
		"--die-with-parent",
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--proc", "/proc",
		"--tmpfs", "/tmp",
	}
	if !policy.Network().Enabled {
		args = append(args, "--unshare-net")
	}
	for _, root := range policy.WritableRoots() {
		args = append(args, "--bind", root, root)
	}
	args = append(args, "--chdir", req.Dir, "--", req.CommandName)
	args = append(args, req.CommandArgs...)
	return args
}

func buildBubblewrapNetworkOnlyArgs(req CommandRequest) []string {
	args := []string{
		"--die-with-parent",
		"--bind", "/", "/",
		"--unshare-net",
		"--chdir", req.Dir,
		"--",
		req.CommandName,
	}
	args = append(args, req.CommandArgs...)
	return args
}
