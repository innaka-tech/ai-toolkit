//go:build !windows

package ops

import (
	"context"
	"os/exec"
	"syscall"
)

// shellCommand runs a command line through sh.
func shellCommand(ctx context.Context, line string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", line)
}

// killTree puts the command in its own process group and makes cancellation kill the whole
// group, so nothing the agent started keeps running after a timeout.
func killTree(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}

// endTree kills what is left of the command's process group after it exited. Processes that
// moved to a process group of their own (setsid) are out of reach; that is documented.
func endTree(c *exec.Cmd) {
	if c.Process != nil {
		syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
