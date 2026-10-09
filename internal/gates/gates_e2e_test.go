package gates_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var bin string

func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "aitk-bin")
	name := "aitk"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin = filepath.Join(dir, name)
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/aitk").CombinedOutput(); err != nil {
		panic(string(out))
	}
	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	os.Setenv("AITK_TOOL", "claude-code")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func sh(t *testing.T, dir string, name string, args ...string) (string, error) {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return string(out), err
}

func must(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	out, err := sh(t, dir, name, args...)
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return out
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func check() string {
	if runtime.GOOS == "windows" {
		return "if exist ok.txt (exit 0) else (exit 1)"
	}
	return "test -f ok.txt"
}

// project: git repo + aitk init (committed) + hooks installed.
func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must(t, dir, "git", "init", "-q", "-b", "main")
	must(t, dir, "git", "config", "user.email", "t@example.com")
	must(t, dir, "git", "config", "user.name", "t")
	must(t, dir, bin, "init", "--check", check())
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: init")
	must(t, dir, bin, "hooks", "install")
	return dir
}

func TestCommitGates(t *testing.T) {
	dir := project(t)

	token := "gh" + "p_" + "aB3dE5fG7hI9jK1lM3nO5pQ7rS9tU1vW3xY5" // built at runtime; no literal secret in source
	write(t, dir, "config.env", "GITHUB_TOKEN="+token+"\n")
	must(t, dir, "git", "add", "config.env")
	out, err := sh(t, dir, "git", "commit", "-qm", "feat: add config")
	if err == nil || !strings.Contains(out, "github-token") || strings.Contains(out, token) {
		t.Fatalf("secret commit must be rejected with a redacted finding:\n%s", out)
	}
	write(t, dir, "config.env", "GITHUB_TOKEN=${GITHUB_TOKEN}\n")
	must(t, dir, "git", "add", "config.env")

	out, err = sh(t, dir, "git", "commit", "-qm", "added config")
	if err == nil || !strings.Contains(out, "Conventional Commit") {
		t.Fatalf("bad message must be rejected:\n%s", out)
	}

	must(t, dir, bin, "task", "new", "Configure env", "--start")
	must(t, dir, "git", "commit", "-qm", "feat(config): read token from env")
	msg := must(t, dir, "git", "log", "-1", "--format=%B")
	if !strings.Contains(msg, "AI-Task: T-") || !strings.Contains(msg, "AI-Tool: claude-code") {
		t.Fatalf("trailers missing:\n%s", msg)
	}

	write(t, dir, "docs/ai/tasks/T-zzzz-broken.md", "---\nid: T-zzzz\nstatus: finished\n---\nbody\n")
	must(t, dir, "git", "add", "docs/ai/tasks/T-zzzz-broken.md")
	out, err = sh(t, dir, "git", "commit", "-qm", "docs: add task")
	if err == nil || !strings.Contains(out, "[schema]") {
		t.Fatalf("invalid task must be rejected:\n%s", out)
	}
}

func TestExistingHooksArePreservedAndHooksPathRespected(t *testing.T) {
	dir := t.TempDir()
	must(t, dir, "git", "init", "-q", "-b", "main")
	must(t, dir, "git", "config", "user.email", "t@example.com")
	must(t, dir, "git", "config", "user.name", "t")
	must(t, dir, "git", "config", "core.hooksPath", ".githooks")
	write(t, dir, ".githooks/pre-commit", "#!/bin/sh\necho user-hook-ran > user-hook.txt\n")
	must(t, dir, bin, "init")
	must(t, dir, bin, "hooks", "install")
	hook, _ := os.ReadFile(filepath.Join(dir, ".githooks/pre-commit"))
	if !strings.Contains(string(hook), "user-hook-ran") || !strings.Contains(string(hook), "aitk hook pre-commit") {
		t.Fatalf("hook not merged:\n%s", hook)
	}
	must(t, dir, bin, "hooks", "install") // idempotent
	if b, _ := os.ReadFile(filepath.Join(dir, ".githooks/pre-commit")); strings.Count(string(b), "aitk:begin") != 1 {
		t.Fatal("block duplicated")
	}
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: init")
	if _, err := os.Stat(filepath.Join(dir, "user-hook.txt")); err != nil {
		t.Fatal("user's own hook did not run")
	}
	must(t, dir, bin, "hooks", "uninstall")
	b, _ := os.ReadFile(filepath.Join(dir, ".githooks/pre-commit"))
	if strings.Contains(string(b), "aitk") || !strings.Contains(string(b), "user-hook-ran") {
		t.Fatalf("uninstall wrong:\n%s", b)
	}
	if _, err := os.Stat(filepath.Join(dir, ".githooks/commit-msg")); err == nil {
		t.Fatal("hook created only for aitk should be removed")
	}
}

func TestPrePushRunsCheck(t *testing.T) {
	dir := project(t)
	remote := t.TempDir()
	must(t, remote, "git", "init", "-q", "--bare")
	must(t, dir, "git", "remote", "add", "origin", remote)
	must(t, dir, bin, "task", "new", "Ship", "--start")
	out, err := sh(t, dir, "git", "push", "-q", "origin", "main")
	if err == nil || !strings.Contains(out, "check failed") {
		t.Fatalf("push with failing check must be rejected:\n%s", out)
	}
	write(t, dir, "ok.txt", "1")
	must(t, dir, "git", "push", "-q", "origin", "main")
}

func TestCI(t *testing.T) {
	dir := project(t)
	must(t, dir, bin, "hooks", "uninstall") // simulate commits made without hooks
	base := strings.TrimSpace(must(t, dir, "git", "rev-parse", "HEAD"))
	write(t, dir, "a.txt", "x")
	must(t, dir, "git", "add", "a.txt")
	must(t, dir, "git", "commit", "-qm", "wip stuff")
	out, err := sh(t, dir, bin, "ci", "--base", base)
	if err == nil || !strings.Contains(out, "commit-message") {
		t.Fatalf("ci must flag the bad commit message:\n%s", out)
	}
	must(t, dir, "git", "commit", "-q", "--amend", "-m", "feat: add a")
	must(t, dir, bin, "ci", "--base", base)
}
