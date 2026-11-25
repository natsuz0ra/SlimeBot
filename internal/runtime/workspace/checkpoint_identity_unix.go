//go:build unix

package workspace

import (
	"fmt"
	"os"
	"syscall"
)

func checkpointIdentity(root *os.Root) (string, error) {
	fi, err := root.Stat(".")
	if err != nil {
		return "", err
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("workspace has no stable filesystem identity")
	}
	return fmt.Sprintf("%x:%x", stat.Dev, stat.Ino), nil
}
