package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/innaka-tech/ai-toolkit/v2/internal/ops"
)

const specKitTasks = `# Tasks: Checkout

## Phase 1: Setup
- [ ] T001 Create project structure
- [ ] T002 [P] Configure linting
- [ ] T003 [P] Configure formatting

## Phase 2: User Story 1
- [ ] T004 [US1] Add cart model in src/cart.py
- [x] T005 [P] [US1] Write cart docs
`

func specKitRepo(t *testing.T) string {
	t.Helper()
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "specs/001-checkout/tasks.md", specKitTasks)
	write(t, dir, "specs/001-checkout/spec.md", "# Spec\n")
	write(t, dir, "specs/001-checkout/plan.md", "# Plan\n")
	mustOK(t, aitk(t, dir, "import", "spec-kit"))
	return dir
}

func ids(list any) []string {
	var out []string
	items, _ := list.([]any)
	for _, it := range items {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	return out
}

func TestSpecKitPhasesMarkersAndNext(t *testing.T) {
	dir := specKitRepo(t)
	show := data(aitk(t, dir, "task", "show", "CHECKOUT-T004"))["task"].(map[string]any)
	if !strings.Contains(show["body"].(string), "specs/001-checkout/spec.md") || !strings.Contains(show["body"].(string), "plan.md") {
		t.Fatalf("spec context missing:\n%s", show["body"])
	}
	if deps := show["depends_on"].([]any); len(deps) != 2 || deps[0] != "CHECKOUT-T002" || deps[1] != "CHECKOUT-T003" {
		t.Fatalf("T004 must wait for the parallel tasks before it, got %v", deps)
	}
	if tags := show["tags"].([]any); len(tags) != 1 || tags[0] != "us1" {
		t.Fatalf("story marker must become a tag, got %v", tags)
	}
	nx := data(aitk(t, dir, "task", "next"))
	if got := ids(nx["ready"]); strings.Join(got, ",") != "CHECKOUT-T001" {
		t.Fatalf("only T001 is ready at first, got %v", got)
	}
	// Finish T001: T002 and T003 become ready together (both parallel), T004 still waits.
	finish(t, dir, "CHECKOUT-T001")
	if got := ids(data(aitk(t, dir, "task", "next"))["ready"]); strings.Join(got, ",") != "CHECKOUT-T002,CHECKOUT-T003" {
		t.Fatalf("parallel tasks must be ready after T001, got %v", got)
	}
	// Starting a task with open dependencies warns.
	r := aitk(t, dir, "task", "start", "CHECKOUT-T004")
	if w := r.env["warnings"].([]any); len(w) == 0 || !strings.Contains(w[0].(string), "CHECKOUT-T002") {
		t.Fatalf("expected dependency warning, got %v", r.env["warnings"])
	}
}

// finish starts id, satisfies its criteria, passes the check, and closes it.
func finish(t *testing.T, dir, id string) {
	t.Helper()
	mustOK(t, aitk(t, dir, "task", "start", id))
	mustOK(t, aitk(t, dir, "task", "update", id, "--add-ac", "it works"))
	mustOK(t, aitk(t, dir, "task", "update", id, "--ac-done", "1"))
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "close", "--summary", "done "+id, "--knowledge", "none"))
}

func TestSpecKitWriteBackAndDrift(t *testing.T) {
	dir := specKitRepo(t)
	finish(t, dir, "CHECKOUT-T001")
	if !strings.Contains(read(t, dir, "specs/001-checkout/tasks.md"), "- [x] T001 Create project structure") {
		t.Fatalf("close must check off the Spec Kit item:\n%s", read(t, dir, "specs/001-checkout/tasks.md"))
	}
	// Someone checks T002 in the file without finishing it in aitk: re-import reports drift.
	md := strings.Replace(read(t, dir, "specs/001-checkout/tasks.md"), "- [ ] T002", "- [x] T002", 1)
	write(t, dir, "specs/001-checkout/tasks.md", md)
	r := aitk(t, dir, "import", "spec-kit")
	mustOK(t, r)
	if d, _ := data(r)["checked_in_source_only"].([]any); len(d) != 1 || d[0] != "CHECKOUT-T002" {
		t.Fatalf("drift not reported: %v", data(r))
	}
	// A task done in aitk while its item was unchecked by hand is checked off again on import.
	write(t, dir, "specs/001-checkout/tasks.md", strings.Replace(md, "- [x] T001", "- [ ] T001", 1))
	r = aitk(t, dir, "import", "spec-kit")
	if s, _ := data(r)["synced"].([]any); len(s) != 1 || !strings.Contains(read(t, dir, "specs/001-checkout/tasks.md"), "- [x] T001") {
		t.Fatalf("re-import must sync done tasks back: %v", data(r))
	}
}

func TestBugFixNeedsRegressionTest(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	id := newStarted(t, dir, "Fix rounding in totals", "--ac", "totals round half up", "--tag", "bug")
	write(t, dir, "src/totals.py", "def total(x):\n    return round(x)\n")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	expect(t, aitk(t, dir, "close", "--summary", "fixed", "--knowledge", "none"), 3, "E_DOD_REGRESSION_TEST")
	write(t, dir, "tests/test_totals.py", "def test_half_up():\n    assert total(2.5) == 3\n")
	mustOK(t, aitk(t, dir, "check"))
	r := aitk(t, dir, "close", "--summary", "fixed with a regression test", "--knowledge", "none")
	mustOK(t, r)
	if data(r)["status"] != "done" {
		t.Fatalf("%s should be done: %v", id, data(r))
	}
}

func TestIsTestPath(t *testing.T) {
	for p, want := range map[string]bool{
		"internal/ops/run_test.go": true, "src/cart.test.ts": true, "web/a.spec.tsx": true, "tests/test_x.py": true,
		"test_x.py": true, "spec/models/user_spec.rb": true, "src/test/java/CartTest.java": true, "CartTests.cs": true,
		"src/latest.java": false, "src/contest.py": false, "specs/001-x/tasks.md": false, "src/cart.ts": false, "README.md": false,
	} {
		if got := ops.IsTestPath(p); got != want {
			t.Errorf("IsTestPath(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestImpactFindsReferencesAndUntestedFiles(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "go.mod", "module example.com/shop\n")
	write(t, dir, "pricing/pricing.go", "package pricing\n\nfunc Total() int { return 1 }\n")
	write(t, dir, "cart/cart.go", "package cart\n\nimport \"example.com/shop/pricing\"\n\nvar _ = pricing.Total\n")
	write(t, dir, "cart/cart_test.go", "package cart\n")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "shop")
	run(t, dir, "git", "checkout", "-qb", "change")
	write(t, dir, "pricing/pricing.go", "package pricing\n\nfunc Total() int { return 2 }\n")
	r := aitk(t, dir, "impact")
	mustOK(t, r)
	files := data(r)["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("one changed source file expected: %v", files)
	}
	f := files[0].(map[string]any)
	refs := f["referenced_by"].([]any)
	if len(refs) != 1 || refs[0] != "cart/cart.go" {
		t.Fatalf("cart/cart.go imports pricing: %v", refs)
	}
	if u := data(r)["untested"].([]any); len(u) != 1 || u[0] != "pricing/pricing.go" {
		t.Fatalf("pricing has no tests: %v", data(r)["untested"])
	}
	write(t, dir, "pricing/pricing_test.go", "package pricing\n")
	if u := data(aitk(t, dir, "impact"))["untested"].([]any); len(u) != 0 {
		t.Fatalf("pricing_test.go covers the package now: %v", u)
	}
}

const osvJSON = `{"results":[{"source":{"path":"SRC/package-lock.json","type":"lockfile"},"packages":[
 {"package":{"name":"axios","version":"1.18.1","ecosystem":"npm"},
  "vulnerabilities":[
   {"id":"GHSA-aaaa","affected":[{"package":{"name":"axios","ecosystem":"npm"},"ranges":[{"events":[{"introduced":"0"},{"fixed":"1.19.2"}]}]}]},
   {"id":"GHSA-bbbb","affected":[{"package":{"name":"axios","ecosystem":"npm"},"ranges":[{"events":[{"introduced":"1.13.0"},{"fixed":"1.20.0"}]},{"events":[{"introduced":"2.0.0"},{"fixed":"2.0.3"}]}]}]}],
  "groups":[{"ids":["GHSA-aaaa"],"max_severity":"7.5"},{"ids":["GHSA-bbbb"],"max_severity":"5.0"}]}]}]}`

func TestAuditCreatesFixTasks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake scanner is a shell script")
	}
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "package-lock.json", "{}\n")
	bin := t.TempDir()
	js := strings.ReplaceAll(osvJSON, "SRC", dir)
	write(t, bin, "osv.json", js)
	write(t, bin, "osv-scanner", "#!/bin/sh\ncase \"$*\" in *json*) cat '"+filepath.Join(bin, "osv.json")+"';; *) echo 'axios 1.18.1 vulnerable';; esac\nexit 1\n")
	os.Chmod(filepath.Join(bin, "osv-scanner"), 0o755)
	for _, tool := range []string{"git", "sh", "cat"} {
		if p, err := exec.LookPath(tool); err == nil {
			os.Symlink(p, filepath.Join(bin, tool))
		}
	}
	t.Setenv("PATH", bin)
	r := aitk(t, dir, "audit", "--tasks")
	expect(t, r, 1, "E_AUDIT_FAILED")
	ft := data(r)["fix_tasks"].(map[string]any)
	created := ft["created"].([]any)
	if len(created) != 1 || created[0] != "VULN-npm-axios" {
		t.Fatalf("one task per vulnerable package: %v", ft)
	}
	show := data(aitk(t, dir, "task", "show", "VULN-npm-axios"))["task"].(map[string]any)
	if !strings.Contains(show["title"].(string), "to 1.20.0 or later (2 advisories)") || !strings.Contains(show["body"].(string), "package-lock.json") {
		t.Fatalf("task must name the fixed version and source: %v / %s", show["title"], show["body"])
	}
	r = aitk(t, dir, "audit", "--tasks")
	if ft := data(r)["fix_tasks"].(map[string]any); len(ft["created"].([]any)) != 0 || len(ft["already_open"].([]any)) != 1 {
		t.Fatalf("a second audit must not duplicate open tasks: %v", ft)
	}
}
