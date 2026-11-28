package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func gitRepo(t *testing.T) *Project {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	p, err := Repo(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGitRelAndAitkRel(t *testing.T) {
	outside := t.TempDir()
	cases := []struct {
		name  string
		setup func(root string)
		dir   string
		want  string
	}{
		{"plain", func(root string) { os.MkdirAll(filepath.Join(root, "docs/ai"), 0o755) }, DocsAI, "docs/ai"},
		{"missing", func(root string) {}, ADRDir, "docs/adr"},
		{"docs is a link", func(root string) {
			os.MkdirAll(filepath.Join(root, "documentation/ai"), 0o755)
			os.Symlink("documentation", filepath.Join(root, "docs"))
		}, DocsAI, "documentation/ai"},
		{"missing behind a link", func(root string) {
			os.MkdirAll(filepath.Join(root, "documentation"), 0o755)
			os.Symlink("documentation", filepath.Join(root, "docs"))
		}, ADRDir, "documentation/adr"},
		{"docs/ai is a link", func(root string) {
			os.MkdirAll(filepath.Join(root, "docs"), 0o755)
			os.MkdirAll(filepath.Join(root, "aidocs"), 0o755)
			os.Symlink("../aidocs", filepath.Join(root, "docs/ai"))
		}, DocsAI, "aidocs"},
		{"link out of the repository", func(root string) { os.Symlink(outside, filepath.Join(root, "docs")) }, DocsAI, "docs/ai"},
		{"link to the root", func(root string) {
			os.MkdirAll(filepath.Join(root, "docs"), 0o755)
			os.Symlink("..", filepath.Join(root, "docs/ai"))
		}, DocsAI, "docs/ai"},
	}
	for _, c := range cases {
		p := gitRepo(t)
		c.setup(p.Root)
		if got := p.GitRel(c.dir); got != c.want {
			t.Errorf("%s: GitRel(%s) = %q, want %q", c.name, c.dir, got, c.want)
		}
		if c.want != c.dir {
			if got := p.AitkRel(c.want + "/x.md"); got != c.dir+"/x.md" {
				t.Errorf("%s: AitkRel(%s/x.md) = %q", c.name, c.want, got)
			}
		}
		if got := p.AitkRel("src/main.go"); got != "src/main.go" {
			t.Errorf("%s: AitkRel changed an ordinary path: %q", c.name, got)
		}
	}
}
