package cli

import (
	"runtime"
	"testing"
)

// docs/ as a symlink to another directory inside the repository: git tracks the files under
// the target, so aitk's own writes there must not look like code changes.
func TestCloseWorksWhenDocsIsASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := repo(t)
	initRepo(t, dir)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: init")
	run(t, dir, "sh", "-c", "mv docs documentation && ln -s documentation docs && git add -A && git commit -qm 'chore: move docs'")
	id := data(aitk(t, dir, "task", "new", "Small", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "task", "update", id, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	// aitk's own files under documentation/ai are not code changes: the diff stays lite.
	b := data(aitk(t, dir, "brief"))
	if b["profile"] != "lite" || len(b["changed_files"].([]any)) != 1 {
		t.Fatalf("aitk's own files behind the docs symlink must not count as changes: %v %v", b["profile"], b["changed_files"])
	}

	// A code change after the check still makes it stale.
	write(t, dir, "later.txt", "x\n")
	expect(t, aitk(t, dir, "close", "--summary", "small", "--knowledge", "none"), 3, "E_DOD_CHECK_STALE")

	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "close", "--summary", "small", "--knowledge", "none"))
	if st := data(aitk(t, dir, "task", "show", id))["task"].(map[string]any)["status"].(string); st != "done" {
		t.Fatalf("close must finish the task in a repository whose docs/ is a symlink, got %s", st)
	}
}
