package client

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func agentCommand(path string, args []string) (*exec.Cmd, error) {
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cmd", ".bat":
		words := append([]string{path}, args...)
		for i, word := range words {
			if strings.ContainsAny(word, "\"%\x00\r\n") {
				return nil, fmt.Errorf("Windows batch command arguments cannot contain quotes, percent signs or newlines")
			}
			words[i] = `"` + word + `"`
		}
		cmd = exec.Command("cmd.exe")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW,
			CmdLine: `cmd.exe /d /v:off /s /c "` + strings.Join(words, " ") + `"`,
		}
	}
	return cmd, nil
}

func killAgentProcess(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
}
