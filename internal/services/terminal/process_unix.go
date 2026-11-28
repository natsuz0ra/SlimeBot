//go:build !windows

package terminal

import (
	"os"
	"syscall"
)

func processCleanup(p *os.Process) func() {
	return func() {
		// The PTY child is a session leader and has its own process group.
		_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
		_ = p.Kill()
	}
}
