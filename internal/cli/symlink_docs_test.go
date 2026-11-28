package cli

import (
	"runtime"
	"strings"
	"testing"
)

// Two layouts where git tracks aitk's files under a symlink's target: docs -> documentation,
// and docs/ai -> ../aidocs. Pathspecs through the link are refused by git.
var docsLayouts = map[string]string{
	"docs link":    "mv docs documentation && ln -s documentation docs",
	"docs/ai link": "mv docs/ai aidocs && ln -s ../aidocs docs/ai",
}

func symlinkedRepo(t *testing.T, layout string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := repo(t)
	initRepo(t, dir)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: init")
	run(t, dir, "sh", "-c", docsLayouts[layout]+" && git add -A && git commit -qm 'chore: move docs'")
	return dir
}

// close works, aitk's own files are not code changes, and a real change still makes the check stale.
func TestCloseWorksWhenDocsIsASymlink(t *testing.T) {
	for layout := range docsLayouts {
		dir := symlinkedRepo(t, layout)
		id := data(aitk(t, dir, "task", "new", "Small", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
		write(t, dir, "ok.txt", "1")
		mustOK(t, aitk(t, dir, "task", "update", id, "--ac-done", "1"))
		mustOK(t, aitk(t, dir, "check"))
		b := data(aitk(t, dir, "brief"))
		if b["profile"] != "lite" || len(b["changed_files"].([]any)) != 1 {
			t.Fatalf("%s: aitk's own files behind the link must not count as changes: %v %v", layout, b["profile"], b["changed_files"])
		}
		write(t, dir, "later.txt", "x\n")
		expect(t, aitk(t, dir, "close", "--summary", "small", "--knowledge", "none"), 3, "E_DOD_CHECK_STALE")
		mustOK(t, aitk(t, dir, "check"))
		mustOK(t, aitk(t, dir, "close", "--summary", "small", "--knowledge", "none"))
		if st := data(aitk(t, dir, "task", "show", id))["task"].(map[string]any)["status"].(string); st != "done" {
			t.Fatalf("%s: close must finish the task, got %s", layout, st)
		}
	}
}

// doctor keeps working behind the link: merge rules name the tracked paths, a broken task file
// is restored from git, and merge conflicts are resolved.
func TestDoctorWorksWhenDocsIsASymlink(t *testing.T) {
	for layout, target := range map[string]string{"docs link": "documentation/ai", "docs/ai link": "aidocs"} {
		dir := symlinkedRepo(t, layout)
		aitk(t, dir, "doctor", "--fix")
		if out := run(t, dir, "git", "check-attr", "merge", "--", target+"/knowledge/_inbox.md"); !strings.Contains(out, "union") {
			t.Fatalf("%s: knowledge files must keep union merge behind the link: %s", layout, out)
		}
		id := data(aitk(t, dir, "task", "new", "Keep me", "--ac", "works"))["id"].(string)
		run(t, dir, "git", "add", "-A")
		run(t, dir, "git", "commit", "-qm", "chore: task")
		f := strings.TrimSpace(run(t, dir, "sh", "-c", "ls docs/ai/tasks/"+id+"-*.md"))
		run(t, dir, "sh", "-c", "sed -i.bak '2s/.*/id: [unclosed/' "+f+" && rm -f "+f+".bak")
		run(t, dir, "sh", "-c", "printf '<<<<<<< ours\\n- a\\n=======\\n- b\\n>>>>>>> theirs\\n' >> docs/ai/knowledge/_inbox.md")
		r := aitk(t, dir, "doctor", "--fix")
		if !strings.Contains(r.stdout, "restored") {
			t.Fatalf("%s: a broken task file must be restored from git:\n%s", layout, r.stdout)
		}
		if strings.Contains(read(t, dir, "docs/ai/knowledge/_inbox.md"), "<<<<<<<") {
			t.Fatalf("%s: merge conflicts behind the link must be resolved:\n%s", layout, r.stdout)
		}
		if title := data(aitk(t, dir, "task", "show", id))["task"].(map[string]any)["title"]; title != "Keep me" {
			t.Fatalf("%s: the restored task keeps its title, got %v", layout, title)
		}
	}
}

// A symlinked CLAUDE.md (a common way to share AGENTS.md) is aitk's file, not a code change.
func TestSymlinkedClaudeMdDoesNotMakeTheCheckStale(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := repo(t)
	initRepo(t, dir)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: init")
	id := data(aitk(t, dir, "task", "new", "Small", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "task", "update", id, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	run(t, dir, "rm", "-f", "CLAUDE.md")
	run(t, dir, "ln", "-s", "AGENTS.md", "CLAUDE.md")
	mustOK(t, aitk(t, dir, "close", "--summary", "small", "--knowledge", "none"))
}
