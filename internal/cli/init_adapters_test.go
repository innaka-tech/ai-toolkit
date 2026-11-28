package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// aitk init installs the project files for the AI tools it detects, so every agent starts
// from aitk brief with no extra step; those files are tool configuration, not code changes.
func TestInitInstallsAdaptersForDetectedTools(t *testing.T) {
	dir := repo(t)
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755) // Claude Code is used here
	initRepo(t, dir)
	for _, f := range []string{".mcp.json", ".claude/settings.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("aitk init must install %s for a detected tool: %v", f, err)
		}
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "chore: aitk init")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	r := aitk(t, dir, "close", "--summary", "Tiny fix", "--knowledge", "none")
	mustOK(t, r)
	if data(r)["profile"] != "lite" {
		t.Fatalf("adapter files must not count as code changes: %v", data(r))
	}

	other := repo(t)
	os.MkdirAll(filepath.Join(other, ".claude"), 0o755)
	mustOK(t, aitk(t, other, "init", "--no-adapters"))
	if _, err := os.Stat(filepath.Join(other, ".mcp.json")); err == nil {
		t.Fatal("--no-adapters must skip the AI tools' files")
	}
}
