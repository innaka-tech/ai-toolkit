package handoff

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
)

// Same-second handoffs for one task keep their write order even when every file has the same
// mtime (as after a git checkout), and -10 is newer than -9.
func TestListOrdersSameSecondHandoffsByWriteOrder(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	p, err := project.Repo(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := "2026-11-28T10:00:00Z"
	var files []string
	for _, outcome := range []string{"progress", "progress", "done", "progress", "progress", "progress", "progress", "progress", "progress", "blocked"} {
		h, err := Write(p, Meta{Task: "T-abcd", At: at, By: "t", Outcome: outcome, TaskStatus: "in_progress"}, "x\n")
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, h.File)
	}
	same := time.Unix(1_800_000_000, 0)
	for _, f := range files {
		if err := os.Chtimes(filepath.Join(dir, f), same, same); err != nil {
			t.Fatal(err)
		}
	}
	got := List(p)
	if len(got) != 10 || got[0].File != files[9] || got[0].Outcome != "blocked" || got[9].File != files[0] {
		var names []string
		for _, h := range got {
			names = append(names, filepath.Base(h.File))
		}
		t.Fatalf("newest first by write order, got %v", names)
	}
}

// Task ids that share a prefix (AUTH-1 and AUTH-1-3) in one second, among more handoffs than
// one insertion-sort block, still give AUTH-1's newest close first.
func TestListWithPrefixSharingIDsStaysOrdered(t *testing.T) {
	for _, before := range []int{0, 18, 19, 25} {
		dir := t.TempDir()
		if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		p, _ := project.Repo(dir)
		write := func(task, at, status string) string {
			h, err := Write(p, Meta{Task: task, At: at, By: "t", Outcome: "progress", TaskStatus: status}, "x\n")
			if err != nil {
				t.Fatal(err)
			}
			return h.File
		}
		var files []string
		for i := 0; i < before; i++ {
			files = append(files, write("T-fill", time.Unix(1_700_000_000+int64(i), 0).UTC().Format("2006-01-02T15:04:05Z"), "in_progress"))
		}
		at := "2026-11-28T10:00:00Z"
		files = append(files, write("AUTH-1", at, "in_progress"), write("AUTH-1", at, "in_review"), write("AUTH-1-3", at, "in_progress"))
		same := time.Unix(1_800_000_000, 0)
		for _, f := range files {
			os.Chtimes(filepath.Join(dir, f), same, same)
		}
		for _, h := range List(p) {
			if h.Task == "AUTH-1" {
				if h.TaskStatus != "in_review" {
					t.Fatalf("before=%d: AUTH-1's newest handoff must be its second close, got %s", before, filepath.Base(h.File))
				}
				break
			}
		}
	}
}
