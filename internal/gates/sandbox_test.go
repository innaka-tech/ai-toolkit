package gates_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestReadOnlyGitDir reproduces agent sandboxes (e.g. Codex workspace-write) where .git
// is read-only: aitk must keep working through the in-tree fallback and still see the
// active task that an unsandboxed tool set.
func TestReadOnlyGitDir(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	dir := project(t)
	must(t, dir, bin, "task", "new", "Sandboxed work", "--ac", "file exists", "--start")
	gitDir := filepath.Join(dir, ".git")
	must(t, dir, "chmod", "-R", "a-w", gitDir)
	t.Cleanup(func() { sh(t, dir, "chmod", "-R", "u+w", gitDir) })

	write(t, dir, "ok.txt", "1")
	for _, args := range [][]string{{"check"}, {"task", "update", "--ac-done", "1"}, {"close", "--summary", "done in a sandbox", "--knowledge", "none"}} {
		if out, err := sh(t, dir, bin, args...); err != nil {
			t.Fatalf("aitk %v failed with a read-only .git:\n%s", args, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".aitk", ".gitignore")); err != nil {
		t.Fatal("fallback state dir is not self-ignoring")
	}
	if st := must(t, dir, "git", "status", "--porcelain", "--untracked-files=all"); strings.Contains(st, ".aitk") {
		t.Fatalf(".aitk shows up in git status:\n%s", st)
	}
	list := must(t, dir, bin, "task", "list", "--all")
	if !strings.Contains(list, "done") {
		t.Fatalf("task not done:\n%s", list)
	}
}
