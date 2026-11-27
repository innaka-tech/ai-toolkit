package profile

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitFile(t *testing.T, dir, name string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", name)
	git(t, dir, "commit", "-q", "-m", name)
}

func paths(cs []Change) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Path)
	}
	sort.Strings(out)
	return out
}

// A clone whose local main is older than origin/main: merged work on origin/main must not
// count as the current branch's change (it inflated sizes and could make a task strict).
func TestDiffUsesTheFresherDefaultBranchRef(t *testing.T) {
	root := t.TempDir()
	upstream, clone := filepath.Join(root, "up"), filepath.Join(root, "clone")
	git(t, root, "init", "-q", "-b", "main", upstream)
	commitFile(t, upstream, "a.go")
	git(t, root, "clone", "-q", upstream, clone)
	commitFile(t, upstream, "merged/one.go") // merged on the remote after the clone
	commitFile(t, upstream, "merged/two.go")
	git(t, clone, "fetch", "-q", "origin")
	git(t, clone, "switch", "-q", "-c", "feature", "origin/main") // local main stays behind
	commitFile(t, clone, "feature.go")

	p := &project.Project{Root: clone, Config: project.Config{}}
	p.Config.Project.DefaultBranch = "main"
	if got := paths(Diff(p)); len(got) != 1 || got[0] != "feature.go" {
		t.Fatalf("stale local main: want only feature.go, got %v", got)
	}

	// Local main ahead of origin/main (unpushed work merged locally): the local merge base wins.
	git(t, clone, "switch", "-q", "main")
	git(t, clone, "merge", "-q", "--ff-only", "origin/main")
	commitFile(t, clone, "local.go")
	git(t, clone, "switch", "-q", "-c", "feature2")
	commitFile(t, clone, "feature2.go")
	if got := paths(Diff(p)); len(got) != 1 || got[0] != "feature2.go" {
		t.Fatalf("local main ahead: want only feature2.go, got %v", got)
	}
}
