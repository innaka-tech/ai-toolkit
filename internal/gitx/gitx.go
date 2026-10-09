// Package gitx wraps the git CLI.
package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Run executes git in dir and returns trimmed stdout.
func Run(dir string, args ...string) (string, error) {
	out, err := RunRaw(dir, args...)
	return strings.TrimSpace(string(out)), err
}

// RunRaw executes git in dir and returns raw stdout.
func RunRaw(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Toplevel returns the repository root containing dir.
func Toplevel(dir string) (string, error) { return Run(dir, "rev-parse", "--show-toplevel") }

// GitDir returns the absolute per-worktree git directory.
func GitDir(dir string) (string, error) { return Run(dir, "rev-parse", "--absolute-git-dir") }

// CommonDir returns the absolute git directory shared by all worktrees.
func CommonDir(dir string) (string, error) {
	return Run(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
}

// Head returns the current commit, or "" in a repository without commits.
func Head(dir string) string {
	s, err := Run(dir, "rev-parse", "--verify", "-q", "HEAD")
	if err != nil {
		return ""
	}
	return s
}

// Branch returns the current branch name, or "" when detached.
func Branch(dir string) string {
	s, _ := Run(dir, "symbolic-ref", "--short", "-q", "HEAD")
	return s
}

// LastCommitDate returns the ISO-8601 committer date of the last commit touching path.
func LastCommitDate(dir, path string) string {
	s, _ := Run(dir, "log", "-1", "--format=%cI", "--", path)
	return s
}
