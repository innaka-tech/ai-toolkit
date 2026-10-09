package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func taskFile(t *testing.T, dir string) string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, "docs/ai/tasks/*.md"))
	if len(m) != 1 {
		t.Fatalf("expected one task file, got %v", m)
	}
	rel, _ := filepath.Rel(dir, m[0])
	return filepath.ToSlash(rel)
}

func TestSelfHealsAgentEditedTask(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	id := newStarted(t, dir, "Invoice export", "--ac", "csv downloads")
	f := taskFile(t, dir)
	// What agents do: hand-edit the YAML with v1-style values and extra fields.
	broken := "---\nid: " + id + "\ntitle: Invoice export\nstatus: DONE\npriority: P1\ncreated: 2026-10-01\nupdated: 2026-10-02 09:30\n---\n\n## Acceptance criteria\n- [x] csv downloads\n"
	write(t, dir, f, broken)
	if d := aitk(t, dir, "doctor"); !strings.Contains(d.stdout, "repairable with aitk doctor --fix") {
		t.Fatalf("doctor should report the damage:\n%s", d.stdout)
	}
	// The next writing command heals it and says so.
	r := aitk(t, dir, "knowledge", "add", "exports use UTF-8 with BOM for Excel")
	mustOK(t, r)
	if ws := r.env["warnings"].([]any); len(ws) == 0 || !strings.Contains(ws[0].(string), "self-healed: repaired "+f) {
		t.Fatalf("expected a self-healed warning, got %v", r.env["warnings"])
	}
	got := read(t, dir, f)
	for _, want := range []string{"status: done", "created: \"2026-10-01T00:00:00Z\"", "updated: \"2026-10-02T09:30:00Z\"", "priority: P1", "- [x] csv downloads"} {
		if !strings.Contains(got, want) {
			t.Fatalf("repaired file lacks %q:\n%s", want, got)
		}
	}
	mustOK(t, aitk(t, dir, "doctor"))
}

func TestRestoresCommittedVersionOfUnparseableTask(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	newStarted(t, dir, "Payments", "--ac", "paid")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: plan")
	f := taskFile(t, dir)
	good := read(t, dir, f)
	write(t, dir, f, "---\nid: [unclosed\ntitle: : :\n---\nbody\n")
	d := aitk(t, dir, "doctor", "--fix")
	mustOK(t, d)
	if read(t, dir, f) != good {
		t.Fatal("committed version not restored")
	}
	q, _ := filepath.Glob(filepath.Join(dir, "docs/ai/_legacy/quarantine/*-broken-*"))
	if len(q) != 1 {
		t.Fatal("broken copy not kept in quarantine")
	}
}

func TestResolvesMergeConflicts(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	id := newStarted(t, dir, "Checkout", "--ac", "pays")
	f := taskFile(t, dir)
	ours := strings.Replace(read(t, dir, f), "status: in_progress", "status: in_progress", 1)
	theirs := strings.Replace(ours, "status: in_progress", "status: done", 1)
	theirs = strings.Replace(theirs, "updated: ", "updated: \"2099-01-01T00:00:00Z\"\nx_old_updated: ", 1)
	write(t, dir, f, "<<<<<<< HEAD\n"+ours+"=======\n"+theirs+">>>>>>> feature\n")
	write(t, dir, "docs/ai/knowledge/general.md", "# Knowledge: general\n\n<<<<<<< HEAD\n- from main <!-- aitk:k date=2026-10-09 -->\n=======\n- from branch <!-- aitk:k date=2026-10-09 -->\n>>>>>>> feature\n")
	mustOK(t, aitk(t, dir, "doctor", "--fix"))
	if got := read(t, dir, f); strings.Contains(got, "<<<<<<<") || !strings.Contains(got, "status: done") {
		t.Fatalf("task conflict not resolved to the newer side:\n%s", got)
	}
	k := read(t, dir, "docs/ai/knowledge/general.md")
	if strings.Contains(k, "=======") || !strings.Contains(k, "from main") || !strings.Contains(k, "from branch") {
		t.Fatalf("knowledge conflict not unioned:\n%s", k)
	}
	if show := aitk(t, dir, "task", "show", id); show.code != 0 {
		t.Fatalf("task unreadable after healing: %s", show.stdout)
	}
}

func TestDoctorFixRepairsAdaptersAndCheck(t *testing.T) {
	dir := repo(t)
	mustOK(t, aitk(t, dir, "init", "--check", "true"))
	cfg := read(t, dir, "aitk.toml")
	cfg = cfg[:strings.Index(cfg, "\n[check]")] + "\n"
	write(t, dir, "aitk.toml", cfg) // check removed; the Makefile from repo() has a test target
	mustOK(t, aitk(t, dir, "adapters", "sync", "--tool", "claude-code"))
	write(t, dir, ".mcp.json", `{"mcpServers": {"db": {"command": "dbmcp"}, "aitk": {"command": "old"}}}`)
	d := aitk(t, dir, "doctor", "--fix")
	mustOK(t, d)
	if !strings.Contains(read(t, dir, ".mcp.json"), `"command": "aitk"`) || !strings.Contains(read(t, dir, ".mcp.json"), "dbmcp") {
		t.Fatalf("adapter drift not repaired:\n%s", read(t, dir, ".mcp.json"))
	}
	if !strings.Contains(read(t, dir, "aitk.toml"), `cmd = "make test"`) {
		t.Fatalf("check command not restored:\n%s", read(t, dir, "aitk.toml"))
	}
	again := aitk(t, dir, "doctor")
	if strings.Contains(again.stdout, "WARN") && strings.Contains(again.stdout, "adapters") {
		t.Fatalf("doctor still warns after fix:\n%s", again.stdout)
	}
	_ = os.Remove
}
