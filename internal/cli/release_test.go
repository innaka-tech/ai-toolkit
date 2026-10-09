package cli

import (
	"strings"
	"testing"
)

func commit(t *testing.T, dir, file, msg string) {
	t.Helper()
	write(t, dir, file, msg+"\n")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", msg)
}

func TestReleaseLifecycle(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "ok.txt", "1")
	write(t, dir, "package.json", "{\n  \"name\": \"shop\",\n  \"version\": \"0.0.0\",\n  \"private\": true\n}\n")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: setup")
	commit(t, dir, "a.txt", "feat(cart): add cart totals\n\nAI-Task: T-k3m9")
	commit(t, dir, "b.txt", "fix: round money to two decimals")
	commit(t, dir, "c.txt", "docs: explain setup")

	dry := aitk(t, dir, "release", "--dry-run")
	mustOK(t, dry)
	if v := data(dry)["version"]; v != "0.1.0" {
		t.Fatalf("first release version = %v", v)
	}
	r := aitk(t, dir, "release", "--tag")
	mustOK(t, r)
	cl := read(t, dir, "CHANGELOG.md")
	for _, want := range []string{"## [Unreleased]", "## [0.1.0]", "### Added", "**cart:** add cart totals", "T-k3m9", "### Fixed", "round money"} {
		if !strings.Contains(cl, want) {
			t.Fatalf("CHANGELOG lacks %q:\n%s", want, cl)
		}
	}
	if strings.Contains(cl, "explain setup") {
		t.Fatal("docs commits are not user-facing changes")
	}
	if !strings.Contains(read(t, dir, "package.json"), `"version": "0.1.0"`) || !strings.Contains(read(t, dir, "ai-state.json"), `"version": "0.1.0"`) {
		t.Fatal("manifest or state version not updated")
	}
	if tags := run(t, dir, "git", "tag", "--list"); !strings.Contains(tags, "v0.1.0") {
		t.Fatal("tag not created")
	}
	expect(t, aitk(t, dir, "release"), 3, "E_RELEASE_NOTHING")

	commit(t, dir, "d.txt", "fix: handle empty cart")
	mustOK(t, aitk(t, dir, "release", "--tag"))
	commit(t, dir, "e.txt", "feat!: new pricing API")
	if v := data(aitk(t, dir, "release", "--tag"))["version"]; v != "0.2.0" {
		t.Fatalf("breaking change in 0.x should bump minor, got %v", v)
	}
	commit(t, dir, "f.txt", "feat: stable")
	if v := data(aitk(t, dir, "release", "--tag", "--bump", "major"))["version"]; v != "1.0.0" {
		t.Fatalf("forced major: %v", v)
	}
	commit(t, dir, "g.txt", "refactor: drop legacy endpoint\n\nBREAKING CHANGE: /v1 removed")
	if v := data(aitk(t, dir, "release", "--tag"))["version"]; v != "2.0.0" {
		t.Fatalf("breaking after 1.0 should bump major, got %v", v)
	}
	commit(t, dir, "h.txt", "feat: search")
	if v := data(aitk(t, dir, "release", "--tag", "--pre", "rc"))["version"]; v != "2.1.0-rc.1" {
		t.Fatalf("pre-release: %v", v)
	}
	commit(t, dir, "i.txt", "feat: search filters") // a feature during rc stays within 2.1.0
	if v := data(aitk(t, dir, "release", "--tag", "--pre", "rc"))["version"]; v != "2.1.0-rc.2" {
		t.Fatalf("second pre-release: %v", v)
	}
	commit(t, dir, "j.txt", "fix: final polish")
	if v := data(aitk(t, dir, "release", "--tag"))["version"]; v != "2.1.0" {
		t.Fatalf("final release after rc: %v", v)
	}
	cl = read(t, dir, "CHANGELOG.md")
	if strings.Index(cl, "## [0.1.0]") < strings.Index(cl, "## [2.0.0]") || !strings.Contains(cl, "### ⚠ Breaking") {
		t.Fatalf("sections out of order or breaking group missing:\n%s", cl)
	}
}

func TestReleaseGates(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: setup")
	commit(t, dir, "a.txt", "feat: something")
	write(t, dir, "wip.txt", "uncommitted")
	expect(t, aitk(t, dir, "release"), 3, "E_RELEASE_DIRTY")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "fix: wip") // ok.txt missing: the check fails
	expect(t, aitk(t, dir, "release"), 3, "E_RELEASE_CHECK")
}
