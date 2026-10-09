//go:build windows

package ops

import (
	"context"
	"fmt"
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

func processAlive(pid int) bool {
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").Output()
	return err == nil && len(out) > 0 && containsPID(string(out), pid)
}

func containsPID(out string, pid int) bool {
	return len(out) > 0 && (indexOf(out, " "+strconv.Itoa(pid)+" ") >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
