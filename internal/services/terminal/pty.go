package terminal

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/aymanbagabas/go-pty"
)

type PTY interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
	Wait() int
}

type processPTY struct {
	pty.Pty
	cmd       *pty.Cmd
	stop      func()
	closeOnce sync.Once
}

func startPTY(shell, cwd string, cols, rows int) (PTY, error) {
	p, err := pty.New()
	if err != nil {
		return nil, err
	}
	if err := p.Resize(cols, rows); err != nil {
		_ = p.Close()
		return nil, err
	}
	cmd := p.Command(shell)
	cmd.Dir = cwd
	cmd.Env = filteredTerminalEnv(os.Environ())
	if err := cmd.Start(); err != nil {
		_ = p.Close()
		return nil, err
	}
	if unix, ok := p.(pty.UnixPty); ok {
		_ = unix.Slave().Close()
	}
	return &processPTY{Pty: p, cmd: cmd, stop: processCleanup(cmd.Process)}, nil
}

func (p *processPTY) Close() error {
	var err error
	p.closeOnce.Do(func() {
		p.stop()
		err = p.Pty.Close()
	})
	return err
}

func (p *processPTY) Wait() int {
	_ = p.cmd.Wait()
	if p.cmd.ProcessState == nil {
		return -1
	}
	return p.cmd.ProcessState.ExitCode()
}

func filteredTerminalEnv(environ []string) []string {
	env := make([]string, 0, len(environ)+2)
	for _, item := range environ {
		key, _, _ := strings.Cut(item, "=")
		switch strings.ToUpper(key) {
		case "JWT_SECRET", "SLIMEBOT_CLI_TOKEN", "SLIMEBOT_DESKTOP_TOKEN", "TERM", "COLORTERM":
			continue
		}
		env = append(env, item)
	}
	return append(env, "TERM=xterm-256color", "COLORTERM=truecolor")
}

func DetectShell(preferred ...string) string {
	if runtime.GOOS == "windows" {
		preferred = append(preferred, "pwsh.exe", "powershell.exe", "cmd.exe")
	} else {
		preferred = append(preferred, os.Getenv("SHELL"), "/bin/zsh", "/bin/bash", "/bin/sh")
	}
	for _, name := range preferred {
		if name == "" {
			continue
		}
		if path, err := exec.LookPath(name); err == nil {
			if absolute, err := filepath.Abs(path); err == nil {
				return absolute
			}
		}
	}
	return ""
}
