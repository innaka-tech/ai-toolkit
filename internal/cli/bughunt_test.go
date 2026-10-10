package cli

import (
	"io"
	"os"
	"regexp"

	"github.com/innaka-tech/ai-toolkit/v2/internal/slash"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Regression tests for the v2.5 bug hunt: each reproduces a reported defect.

func sensitiveRepo(t *testing.T) string {
	t.Helper()
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), "sensitive_paths = []", `sensitive_paths = ["auth/**"]`, 1))
	write(t, dir, "auth/session.go", "package auth\n")
	write(t, dir, "ok.txt", "1")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: base")
	return dir
}

// Committed work on the default branch still counts toward the task's risk profile.
func TestCommittedSensitiveWorkStaysStrict(t *testing.T) {
	dir := sensitiveRepo(t)
	newStarted(t, dir, "Change login", "--ac", "login works")
	write(t, dir, "auth/login.go", "package auth\n\nfunc Login() {}\n")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "feat: login")
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	r := aitk(t, dir, "close", "--summary", "login", "--knowledge", "none")
	if r.code == 0 && data(r)["status"] == "done" {
		t.Fatalf("a committed change under auth/ must make the task strict, not done: %v", data(r))
	}
}

// Moving a file out of a sensitive path is a sensitive change.
func TestRenameOutOfSensitivePathIsStrict(t *testing.T) {
	dir := sensitiveRepo(t)
	newStarted(t, dir, "Move session", "--ac", "moved")
	run(t, dir, "git", "mv", "auth/session.go", "session.go")
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	r := aitk(t, dir, "close", "--summary", "moved", "--knowledge", "none")
	if r.code == 0 && data(r)["status"] == "done" {
		t.Fatalf("moving a file out of auth/ must be strict: %v", data(r))
	}
}

func TestCancelledTaskCannotBeClosed(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	id := newStarted(t, dir, "Cancel me", "--ac", "never")
	mustOK(t, aitk(t, dir, "task", "update", id, "--status", "cancelled"))
	write(t, dir, "ok.txt", "1")
	aitk(t, dir, "check")
	r := aitk(t, dir, "close", "--summary", "x", "--knowledge", "none")
	if show := data(aitk(t, dir, "task", "show", id))["task"].(map[string]any); show["status"] != "cancelled" {
		t.Fatalf("a cancelled task must stay cancelled: %v / %v", show["status"], r.stdout)
	}
}

// Committing the checked code does not make the check stale.
func TestCommitAfterCheckIsNotStale(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	run(t, dir, "git", "checkout", "-qb", "feat")
	newStarted(t, dir, "Feature", "--ac", "works")
	write(t, dir, "f.txt", "change\n")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	run(t, dir, "git", "add", "f.txt", "ok.txt")
	run(t, dir, "git", "commit", "-qm", "feat: f")
	mustOK(t, aitk(t, dir, "close", "--summary", "f", "--knowledge", "none"))
}

// A symlink re-pointed after the check makes it stale.
func TestSymlinkChangeMakesCheckStale(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "v1/a", "1")
	write(t, dir, "v2/a", "2")
	os.Symlink("v1", filepath.Join(dir, "cur"))
	newStarted(t, dir, "Link", "--ac", "linked")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	os.Remove(filepath.Join(dir, "cur"))
	os.Symlink("v2", filepath.Join(dir, "cur"))
	expect(t, aitk(t, dir, "close", "--summary", "x", "--knowledge", "none"), 3, "E_DOD_CHECK_STALE")
}

// A Markdown setext heading is not a merge conflict.
func TestHealLeavesSetextHeadingsAlone(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	ctx := "Payments service\n=======\n\nHandles money.\n"
	write(t, dir, "docs/ai/project-context.md", ctx)
	mustOK(t, aitk(t, dir, "task", "new", "First task"))
	mustOK(t, aitk(t, dir, "task", "new", "Second task"))
	if got := read(t, dir, "docs/ai/project-context.md"); got != ctx {
		t.Fatalf("self-heal changed a valid file:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs/ai/_legacy/quarantine")); err == nil {
		t.Fatal("nothing should be quarantined")
	}
}

func TestKnowledgeKeepsDifferentNumbers(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	mustOK(t, aitk(t, dir, "knowledge", "add", "Retry timeout is 1.5s for payments.", "--tag", "pay"))
	mustOK(t, aitk(t, dir, "knowledge", "add", "Retry timeout is 15s for payments.", "--tag", "pay"))
	r := aitk(t, dir, "knowledge", "compact")
	mustOK(t, r)
	pay := read(t, dir, "docs/ai/knowledge/pay.md")
	if !strings.Contains(pay, "1.5s") || !strings.Contains(pay, "15s") {
		t.Fatalf("different findings were merged:\n%s", pay)
	}
}

func TestInitDetectsPackedDefaultBranch(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q", "-b", "master")
	run(t, dir, "git", "config", "user.email", "t@example.com")
	run(t, dir, "git", "config", "user.name", "t")
	run(t, dir, "git", "commit", "-q", "--allow-empty", "-m", "init")
	run(t, dir, "git", "pack-refs", "--all")
	mustOK(t, aitk(t, dir, "init"))
	if !strings.Contains(read(t, dir, "aitk.toml"), `default_branch = "master"`) {
		t.Fatalf("packed refs/heads/master not detected:\n%s", read(t, dir, "aitk.toml"))
	}
}

func TestReleaseEditsOnlyThePackageVersion(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "ok.txt", "1")
	cargo := "[package]\nname = \"x\"\nversion = \"0.1.0\"\n\n[dependencies.serde]\nversion = \"1.0.190\"\n"
	write(t, dir, "Cargo.toml", cargo)
	pkg := "{\n\t\"name\": \"x\",\n\t\"version\": \"0.1.0\",\n\t\"files\": [\"a\", \"b\"]\n}\n"
	write(t, dir, "package.json", pkg)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: setup")
	run(t, dir, "git", "tag", "v0.1.0")
	commit(t, dir, "a.txt", "feat: a")
	mustOK(t, aitk(t, dir, "release"))
	got := read(t, dir, "Cargo.toml")
	if !strings.Contains(got, "version = \"0.2.0\"") || !strings.Contains(got, "version = \"1.0.190\"") {
		t.Fatalf("only [package] version may change:\n%s", got)
	}
	if want := strings.Replace(pkg, "0.1.0", "0.2.0", 1); read(t, dir, "package.json") != want {
		t.Fatalf("package.json must be edited in place:\n%s", read(t, dir, "package.json"))
	}
}

func TestReleaseIgnoresNonSemverTagsAndCountsReverts(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "ok.txt", "1")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: setup")
	run(t, dir, "git", "tag", "v1.5.0")
	commit(t, dir, "a.txt", "feat: a")
	run(t, dir, "git", "tag", "v2-beta")
	run(t, dir, "git", "revert", "--no-edit", "HEAD")
	r := aitk(t, dir, "release", "--dry-run")
	mustOK(t, r)
	if d := data(r); d["previous"] != "1.5.0" || d["version"] != "1.6.0" || len(d["commits"].([]any)) != 2 {
		t.Fatalf("a non-SemVer tag must not reset the version, and the revert must be listed: %v", d)
	}
}

func TestUATRejectRefusesCancelledTasks(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	asPerson(t)
	id := newStarted(t, dir, "Cancelled one", "--ac", "x")
	mustOK(t, aitk(t, dir, "task", "update", id, "--status", "cancelled"))
	expect(t, aitk(t, dir, "uat", "reject", id, "--reason", "nope"), 2, "E_USAGE")
}

func TestImportsKeepEveryItem(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	plan := "# Plan\n\n### Task 1: One\n- [ ] Step\n\n### Task 2: Two\n- [ ] Step\n\n### Task 3: Three\n- [ ] Step\n"
	write(t, dir, "docs/superpowers/plans/2026-10-09-customer-account-self-service-portal.md", plan)
	r := aitk(t, dir, "import", "superpowers")
	mustOK(t, r)
	if imp := data(r)["imported"].([]any); len(imp) != 3 {
		t.Fatalf("every task of a long-named plan must be imported: %v", data(r))
	}
	write(t, dir, "todo.md", "- [ ] Alpha\n- [ ] Beta\n\n```\n- [ ] not a task\n```\n- [ ] Gamma\n  - [ ] nested step\n")
	r = aitk(t, dir, "import", "markdown", "todo.md")
	if imp := data(r)["imported"].([]any); len(imp) != 3 {
		t.Fatalf("code blocks and nested steps are not tasks: %v", data(r))
	}
	write(t, dir, "todo.md", "- [ ] Zeta\n- [ ] Alpha\n- [ ] Beta\n\n- [ ] Gamma\n")
	r = aitk(t, dir, "import", "markdown", "todo.md")
	imp := data(r)["imported"].([]any)
	if len(imp) != 1 {
		t.Fatalf("only the new item is imported after an insertion: %v", data(r))
	}
	if show := data(aitk(t, dir, "task", "show", imp[0].(string)))["task"].(map[string]any); show["title"] != "Zeta" {
		t.Fatalf("the new task must be Zeta, got %v", show["title"])
	}
}

func TestJSONForGroupsTyposAndDoubleDash(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	mustOK(t, aitk(t, dir, "task"))
	expect(t, aitk(t, dir, "task", "nosuch"), 2, "E_USAGE")
	r := aitk(t, dir, "knowledge", "search", "--", "--json")
	_ = r // must still be one JSON envelope (checked by aitk())
}

func TestBOMAndNegativeFindings(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, ".mcp.json", "\xef\xbb\xbf{\n  \"mcpServers\": {}\n}\n")
	r := aitk(t, dir, "adapters", "sync", "--tool", "claude-code")
	mustOK(t, r)
	if !strings.Contains(read(t, dir, ".mcp.json"), `"aitk"`) {
		t.Fatal("a .mcp.json with a byte order mark must still get the aitk server")
	}
	newStarted(t, dir, "Review me", "--ac", "x")
	r = aitk(t, dir, "review", "pass", "--findings", "-3")
	if r.code != 2 || !strings.Contains(r.stdout, "negative") {
		t.Fatalf("negative findings need their own message: %s", r.stdout)
	}
}

// Bug hunt round 2: items with the same text, and nested steps.
func TestImportDuplicateTextsAndNestedSteps(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	md := "- [ ] Write tests\n- [ ] Write tests\n- [ ] Ship it\n  - [x] build passes\n  - [ ] notes written\n"
	write(t, dir, "dup.md", md)
	mustOK(t, aitk(t, dir, "import", "markdown", "dup.md"))
	for i := 0; i < 2; i++ {
		if imp := data(aitk(t, dir, "import", "markdown", "dup.md"))["imported"].([]any); len(imp) != 0 {
			t.Fatalf("re-importing must not create tasks: %v", imp)
		}
	}
	ship := data(aitk(t, dir, "task", "show", "DUP-3"))
	crit, _ := ship["criteria"].([]any)
	if len(crit) != 2 || !crit[0].(map[string]any)["done"].(bool) {
		t.Fatalf("nested steps must become the parent's criteria (checked state kept): %v", ship["criteria"])
	}
	// Closing the second "Write tests" checks off the second line, not the first.
	mustOK(t, aitk(t, dir, "task", "start", "DUP-2"))
	mustOK(t, aitk(t, dir, "task", "update", "--add-ac", "done"))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "close", "--summary", "x", "--knowledge", "none"))
	if got := read(t, dir, "dup.md"); !strings.HasPrefix(got, "- [ ] Write tests\n- [x] Write tests\n") {
		t.Fatalf("the second occurrence must be checked off:\n%s", got)
	}
}

// Round 2: an amended commit keeps the task's work in its profile.
func TestAmendKeepsSensitiveWorkStrict(t *testing.T) {
	dir := sensitiveRepo(t)
	newStarted(t, dir, "Amend auth", "--ac", "ok")
	write(t, dir, "auth/x.go", "package auth\n")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-q", "--amend", "--no-edit")
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	if r := aitk(t, dir, "close", "--summary", "x", "--knowledge", "none"); r.code == 0 && data(r)["status"] == "done" {
		t.Fatalf("an amended auth change must stay strict: %v", data(r))
	}
}

// Round 2: a close without a task covers what was committed since the last close.
func TestImplicitCloseSeesCommittedSensitiveWork(t *testing.T) {
	dir := sensitiveRepo(t)
	mustOK(t, aitk(t, dir, "switch", "codex", "--print")) // a handoff to count from
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: aitk")
	write(t, dir, "auth/l.go", "package auth\n")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "feat: l")
	aitk(t, dir, "check")
	if r := aitk(t, dir, "close", "--summary", "x", "--knowledge", "none"); r.code == 0 {
		t.Fatalf("committed auth work must not close as an implicit lite task: %v", data(r))
	}
}

// Round 2: a check recorded while .git/objects is read-only still matches at close.
func TestReadOnlyObjectsDoNotBreakFreshness(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	dir := repo(t)
	initRepo(t, dir)
	newStarted(t, dir, "Read-only objects", "--ac", "ok")
	write(t, dir, "f.txt", "x")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	objects := filepath.Join(dir, ".git", "objects")
	run(t, dir, "chmod", "-R", "a-w", objects)
	defer run(t, dir, "chmod", "-R", "u+w", objects)
	mustOK(t, aitk(t, dir, "check"))
	run(t, dir, "chmod", "-R", "u+w", objects)
	mustOK(t, aitk(t, dir, "close", "--summary", "x", "--knowledge", "none"))
}

func TestBriefNamesMissingActiveTask(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: aitk")
	run(t, dir, "git", "checkout", "-qb", "feat")
	mustOK(t, aitk(t, dir, "task", "new", "Branch task", "--id", "T-br", "--start"))
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: task")
	run(t, dir, "git", "checkout", "-q", "main")
	if b := data(aitk(t, dir, "brief")); b["missing_active_task"] != "T-br" || !strings.Contains(b["markdown"].(string), "T-br does not exist") {
		t.Fatalf("the brief must name the missing active task: %v", b["markdown"])
	}
}

func TestReleaseBumpsCargoWorkspaceAndLabelsReverts(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "ok.txt", "1")
	write(t, dir, "Cargo.toml", "[workspace]\nmembers = [\"a\"]\n\n[workspace.package]\nversion = \"0.3.0\"\n")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: setup")
	run(t, dir, "git", "tag", "v0.3.0")
	commit(t, dir, "a.txt", "feat: a")
	run(t, dir, "git", "revert", "--no-edit", "HEAD")
	commit(t, dir, "b.txt", "fix: b")
	mustOK(t, aitk(t, dir, "release"))
	if !strings.Contains(read(t, dir, "Cargo.toml"), `version = "0.4.0"`) {
		t.Fatalf("workspace version not bumped:\n%s", read(t, dir, "Cargo.toml"))
	}
	if !strings.Contains(read(t, dir, "CHANGELOG.md"), "Revert feat: a") {
		t.Fatalf("a revert must say so in the changelog:\n%s", read(t, dir, "CHANGELOG.md"))
	}
}

// Round 3: the very first close without a task (no handoff yet) still sees committed work.
func TestFirstImplicitCloseSeesCommittedSensitiveWork(t *testing.T) {
	dir := sensitiveRepo(t) // aitk.toml was committed with the base
	write(t, dir, "auth/l.go", "package auth\n")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "feat: l")
	aitk(t, dir, "check")
	if r := aitk(t, dir, "close", "--summary", "changed auth", "--knowledge", "none"); r.code == 0 {
		t.Fatalf("committed auth work must not close as an implicit lite task: %v", data(r))
	}
}

func TestPromptCommandAndTaskDepends(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	if _, err := os.Stat(filepath.Join(dir, "docs/ai/commands/README.md")); err != nil {
		t.Fatal("init must explain project commands")
	}
	list := aitk(t, dir, "prompt")
	mustOK(t, list)
	if !strings.Contains(list.stdout, "aitk-review") {
		t.Fatalf("prompt must list the built-ins: %s", list.stdout)
	}
	r := aitk(t, dir, "prompt", "aitk-plan", "add", "CSV", "export")
	mustOK(t, r)
	if !strings.Contains(data(r)["prompt"].(string), "Plan this work as aitk tasks: add CSV export") {
		t.Fatalf("arguments not filled in: %v", data(r)["prompt"])
	}
	expect(t, aitk(t, dir, "prompt", "nope"), 2, "E_USAGE")
	a := data(aitk(t, dir, "task", "new", "Schema"))["id"].(string)
	b := aitk(t, dir, "task", "new", "API on the schema", "--depends", a)
	mustOK(t, b)
	if deps := data(b)["depends_on"].([]any); len(deps) != 1 || deps[0] != a {
		t.Fatalf("--depends not recorded: %v", data(b))
	}
	expect(t, aitk(t, dir, "task", "new", "Broken", "--depends", "T-none"), 2, "E_TASK_NOT_FOUND")
}

// Every aitk flag a built-in command tells an agent to use must exist.
func TestBuiltinCommandsUseRealFlags(t *testing.T) {
	root := (&app{stdout: io.Discard, stderr: io.Discard}).root()
	re := regexp.MustCompile("`aitk ([a-z][a-z ]*?)((?: --[a-z-]+[^`]*)?)`")
	flagRE := regexp.MustCompile(`--([a-z-]+)`)
	for _, c := range slash.Builtins() {
		for _, m := range re.FindAllStringSubmatch(c.Body, -1) {
			words := strings.Fields(m[1])
			cmd, rest, err := root.Find(words)
			if err != nil || cmd == root {
				t.Errorf("%s: unknown command %q", c.Name, m[0])
				continue
			}
			_ = rest
			for _, f := range flagRE.FindAllStringSubmatch(m[2], -1) {
				if cmd.Flags().Lookup(f[1]) == nil && cmd.InheritedFlags().Lookup(f[1]) == nil {
					t.Errorf("%s: %s has no --%s (%q)", c.Name, cmd.CommandPath(), f[1], m[0])
				}
			}
		}
	}
}

func TestReviewPassByIDWithoutStarting(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	id := data(aitk(t, dir, "task", "new", "Reviewed task", "--ac", "x"))["id"].(string)
	r := aitk(t, dir, "review", "pass", id, "--findings", "0")
	mustOK(t, r)
	show := data(aitk(t, dir, "task", "show", id))["task"].(map[string]any)
	if show["status"] != "todo" || show["workers"] != nil {
		t.Fatalf("recording a review by id must not start the task: %v", show)
	}
}
