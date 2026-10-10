package checkrun

import (
	"runtime"
	"testing"
	"time"
)

// Bug hunt: a background process holding the output pipe must not outlive the timeout.
func TestTimeoutEndsBackgroundProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh syntax")
	}
	start := time.Now()
	r := Run(t.TempDir(), "sleep 30 & sleep 30; true", time.Second, false)
	if !r.TimedOut || time.Since(start) > 10*time.Second {
		t.Fatalf("check ran %s (timed out: %v); the timeout is 1s", time.Since(start), r.TimedOut)
	}
}
