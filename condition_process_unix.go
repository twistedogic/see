//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// configureConditionCommand makes context cancellation terminate the shell's
// process group, not only the shell parent that may have spawned a child.
func configureConditionCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
