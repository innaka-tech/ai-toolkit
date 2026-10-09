// Package checkrun runs the project's check command with a timeout.
package checkrun

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// Result of one run.
type Result struct {
	ExitCode   int
	DurationMs int64
	Output     string // combined stdout+stderr
	TimedOut   bool
}

// Run executes cmd through the platform shell in dir.
func Run(dir, cmd string, timeout time.Duration, stream bool) Result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd", "/C", cmd)
	} else {
		c = exec.CommandContext(ctx, "sh", "-c", cmd)
	}
	c.Dir = dir
	var buf bytes.Buffer
	if stream {
		c.Stdout = ioMulti(&buf, os.Stderr)
		c.Stderr = ioMulti(&buf, os.Stderr)
	} else {
		c.Stdout, c.Stderr = &buf, &buf
	}
	start := time.Now()
	err := c.Run()
	r := Result{DurationMs: time.Since(start).Milliseconds(), Output: buf.String()}
	var ee *exec.ExitError
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		r.TimedOut, r.ExitCode = true, 124
	case errors.As(err, &ee):
		r.ExitCode = ee.ExitCode()
	case err != nil:
		r.ExitCode = 127
		r.Output += "\n" + err.Error()
	}
	return r
}

// Summary keeps the last part of the output (where failures usually are), max n runes.
func Summary(out string, n int) string {
	r := []rune(out)
	if len(r) <= n {
		return out
	}
	return "…" + string(r[len(r)-n+1:])
}
