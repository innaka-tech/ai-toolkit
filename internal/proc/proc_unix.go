//go:build !windows

// Package proc starts commands so that a timeout ends them and everything they started.
package proc

import (
	"context"
	"os/exec"
	"syscall"
)

// Shell runs a command line through sh.
func Shell(ctx context.Context, line string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", line)
}

// KillTree puts the command in its own process group and makes cancellation kill the whole
// group. Processes that move to a session of their own (setsid) are out of reach.
func KillTree(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}

// EndTree kills what is left of the command's process group after it exited.
func EndTree(c *exec.Cmd) {
	if c.Process != nil {
		syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
