//go:build windows

// Package proc starts commands so that a timeout ends them and everything they started.
package proc

import (
	"context"
	"os/exec"
	"strconv"
	"syscall"
)

// Shell runs a command line through cmd.exe exactly as written (no Go argument quoting).
func Shell(ctx context.Context, line string) *exec.Cmd {
	c := exec.CommandContext(ctx, "cmd")
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd /S /C "` + line + `"`}
	return c
}

// KillTree makes cancellation end the whole process tree.
func KillTree(c *exec.Cmd) {
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(c.Process.Pid)).Run()
	}
}

// EndTree ends what is left of the command's process tree after it exited.
func EndTree(c *exec.Cmd) {
	if c.Process != nil {
		exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(c.Process.Pid)).Run()
	}
}
