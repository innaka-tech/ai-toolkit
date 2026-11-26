package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandardsListShowAndProjectOverride(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	list := aitk(t, dir, "standards")
	mustOK(t, list)
	for _, topic := range []string{"character", "sdlc", "security", "testing", "database", "api", "operations"} {
		if !strings.Contains(list.stdout, topic) {
			t.Fatalf("standards must list %s: %s", topic, list.stdout)
		}
	}
	sec := aitk(t, dir, "standards", "security")
	mustOK(t, sec)
	if !strings.Contains(data(sec)["body"].(string), "OWASP") {
		t.Fatalf("security standard body missing: %v", data(sec))
	}
	expect(t, aitk(t, dir, "standards", "nope"), 2, "E_USAGE")

	os.MkdirAll(filepath.Join(dir, "docs/ai/standards"), 0o755)
	os.WriteFile(filepath.Join(dir, "docs/ai/standards/security.md"), []byte("# Our security\n\nStricter rules.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "docs/ai/standards/logistics.md"), []byte("# Logistics domain\n\nAWB numbers are unique.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "docs/ai/standards/Bad Name.md"), []byte("# x\n"), 0o644)
	over := aitk(t, dir, "standards", "security")
	mustOK(t, over)
	if d := data(over); d["source"] != "docs/ai/standards/security.md" || d["title"] != "Our security" {
		t.Fatalf("project file must override the built-in: %v", d)
	}
	l2 := aitk(t, dir, "standards")
	mustOK(t, l2)
	if !strings.Contains(l2.stdout, "logistics") || !strings.Contains(l2.stdout, "lowercase") || strings.Contains(l2.stdout, `"topic":"Bad Name"`) {
		t.Fatalf("project topic missing or bad name not skipped with a warning: %s", l2.stdout)
	}
}

func TestDebtListsShortcutComments(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\n// shortcut: in-memory cache, move to Redis above 1 instance\nvar cache = map[string]string{}\n\nvar s = \"no shortcut: here\"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "job.py"), []byte("x = 1  # shortcut: no retry, add backoff when the API flakes\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "NOTES.md"), []byte("// shortcut: prose in markdown is not debt\n"), 0o644)
	exec.Command("git", "-C", dir, "add", "main.go").Run()
	r := aitk(t, dir, "debt")
	mustOK(t, r)
	items := r.env["data"].([]any)
	if len(items) != 2 {
		t.Fatalf("want 2 shortcuts (tracked and untracked, not markdown or strings), got %v", items)
	}
	first := items[0].(map[string]any)
	if first["file"] != "job.py" && first["file"] != "main.go" {
		t.Fatalf("unexpected file: %v", first)
	}
	for _, it := range items {
		m := it.(map[string]any)
		if m["file"] == "main.go" && (m["line"].(float64) != 3 || m["text"] != "in-memory cache, move to Redis above 1 instance") {
			t.Fatalf("wrong location or text: %v", m)
		}
	}
}

func TestBriefRulesCarryTheCharacter(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	r := aitk(t, dir, "brief")
	mustOK(t, r)
	md, _ := data(r)["markdown"].(string)
	if !strings.Contains(md, "Smallest change that fully works") || !strings.Contains(md, "Skipped: and Risk:") {
		t.Fatalf("brief rules must carry the character: %s", md)
	}
}
