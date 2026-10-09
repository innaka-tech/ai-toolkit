//go:build windows

package ops

import (
	"context"
	"os/exec"
	"strconv"
	"syscall"
)

// shellCommand runs a command line through cmd.exe exactly as written (no Go argument quoting).
func shellCommand(ctx context.Context, line string) *exec.Cmd {
	c := exec.CommandContext(ctx, "cmd")
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd /S /C "` + line + `"`}
	return c
}

// killTree makes cancellation end the whole process tree.
func killTree(c *exec.Cmd) {
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(c.Process.Pid)).Run()
	}
}

// endTree ends what is left of the command's process tree after it exited.
func endTree(c *exec.Cmd) {
	if c.Process != nil {
		exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(c.Process.Pid)).Run()
	}
}
