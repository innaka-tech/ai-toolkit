package audit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
)

// Legacy files moved aside by migrate are skipped by the secret scan, also behind a docs symlink.
func TestScanSkipsLegacyBehindADocsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	key := "AKIA" + "IOSFODNN7EXAMPLQ" // built at runtime; no literal secret in source
	os.MkdirAll(filepath.Join(dir, "documentation/ai/_legacy"), 0o755)
	os.WriteFile(filepath.Join(dir, "documentation/ai/_legacy/old.md"), []byte("aws_access_key_id = "+key+"\n"), 0o644)
	os.Symlink("documentation", filepath.Join(dir, "docs"))
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	p, err := project.Repo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fs := ScanTrackedFiles(p); len(fs) != 0 {
		t.Fatalf("legacy files behind the docs symlink must be skipped: %+v", fs)
	}
}
