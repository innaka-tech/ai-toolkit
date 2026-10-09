package cli

import (
	"strings"
	"testing"
)

// Regressions for the independent review of v2.0.2 (numbers refer to its findings).

func TestReview1CompactKeepsHandWrittenTopicFile(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "docs/ai/knowledge/general.md", "# General\n\nhand-written prose that must stay\n\n- existing entry\n")
	mustOK(t, aitk(t, dir, "knowledge", "add", "new finding"))
	mustOK(t, aitk(t, dir, "knowledge", "compact"))
	g := read(t, dir, "docs/ai/knowledge/general.md")
	for _, want := range []string{"# General", "hand-written prose that must stay", "- existing entry", "new finding"} {
		if !strings.Contains(g, want) {
			t.Fatalf("general.md lost %q:\n%s", want, g)
		}
	}
}

func TestReview2BlankLineInsideEntrySurvives(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "docs/ai/knowledge/_inbox.md", read(t, dir, "docs/ai/knowledge/_inbox.md")+"Some prose someone wrote in the inbox.\n\n")
	mustOK(t, aitk(t, dir, "knowledge", "add", "para one\n\npara two must survive"))
	mustOK(t, aitk(t, dir, "knowledge", "compact"))
	all := readAll(t, dir, "docs/ai/knowledge")
	if !strings.Contains(all, "para two must survive") || !strings.Contains(all, "Some prose someone wrote in the inbox.") {
		t.Fatalf("text lost:\n%s", all)
	}
	hits := aitk(t, dir, "knowledge", "search", "para two")
	if arr := hits.env["data"].([]any); len(arr) == 0 || !strings.Contains(arr[0].(map[string]any)["text"].(string), "para two must survive") {
		t.Fatalf("multi-paragraph entry not parsed as one entry: %v", arr)
	}
}

func TestReview3CompactKeepsHeadingsAndComments(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "docs/ai/knowledge/db.md", "# DB\n\n## Postgres notes\n<!-- keep this comment -->\n- entry one\n")
	mustOK(t, aitk(t, dir, "knowledge", "add", "use connection pooling", "--tag", "db"))
	mustOK(t, aitk(t, dir, "knowledge", "compact"))
	db := read(t, dir, "docs/ai/knowledge/db.md")
	for _, want := range []string{"## Postgres notes", "<!-- keep this comment -->", "- entry one", "use connection pooling"} {
		if !strings.Contains(db, want) {
			t.Fatalf("db.md lost %q:\n%s", want, db)
		}
	}
}

func sensitive(t *testing.T, dir, glob string) {
	t.Helper()
	cfg := read(t, dir, "aitk.toml")
	write(t, dir, "aitk.toml", strings.Replace(cfg, "sensitive_paths = []", `sensitive_paths = ["`+glob+`"]`, 1))
}

func TestReview4NonASCIIPathsAreSeen(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	sensitive(t, dir, "secrets/**")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "aitk init")
	newStarted(t, dir, "Touch secrets", "--ac", "done")
	write(t, dir, "secrets/clé.txt", "v1")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	if p := data(aitk(t, dir, "brief"))["profile"]; p != "strict" {
		t.Fatalf("non-ASCII sensitive path not detected: profile %v", p)
	}
	write(t, dir, "secrets/clé.txt", "v2") // edit after the passing check
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	expect(t, aitk(t, dir, "close", "--summary", "s", "--knowledge", "none"), 3, "E_DOD_CHECK_STALE")
}

func TestReview5PinnedLiteCannotEscapeSensitivePaths(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	sensitive(t, dir, "auth/**")
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "aitk init")
	newStarted(t, dir, "Tweak login", "--profile", "lite")
	write(t, dir, "auth/login.go", "package auth\n")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	r := aitk(t, dir, "close", "--summary", "s", "--knowledge", "none")
	if r.code == 0 || data(aitk(t, dir, "brief"))["profile"] != "strict" {
		t.Fatalf("pinned lite escaped sensitive path: exit %d %v", r.code, r.stdout)
	}
}

func TestReview6ReviewMustBeIndependentOfImplementer(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	sensitive(t, dir, "db/**")
	reviewOnly(t, dir)
	t.Setenv("AITK_TOOL", "human")
	id := newStarted(t, dir, "Migration", "--ac", "applies")
	t.Setenv("AITK_TOOL", "claude-code") // implementer differs from creator
	mustOK(t, aitk(t, dir, "task", "start", id))
	write(t, dir, "db/001.sql", "create table x(id int);\n")
	write(t, dir, "ok.txt", "1")
	body := "Impact: new table.\nSecurity (STRIDE): none.\nRollback: drop table.\nValidation: migration test."
	f := strings.TrimSpace(run(t, dir, "sh", "-c", "ls docs/ai/tasks/*.md"))
	write(t, dir, f, strings.Replace(read(t, dir, f), "(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)", body, 1))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "1"))
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	mustOK(t, aitk(t, dir, "check"))
	r := aitk(t, dir, "close", "--summary", "s", "--knowledge", "none")
	mustOK(t, r)
	if data(r)["status"] != "in_review" {
		t.Fatalf("self-review by the implementer must not reach done: %v", data(r)["status"])
	}
}

func TestReview7UpdateCannotSetDone(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	id := newStarted(t, dir, "Never met", "--ac", "never met")
	for _, st := range []string{"done", "implemented", "in_review"} {
		expect(t, aitk(t, dir, "task", "update", id, "--status", st), 2, "E_USAGE")
	}
	mustOK(t, aitk(t, dir, "task", "update", id, "--status", "blocked"))
}

func TestReview8NoCommitsStaleCheckDetected(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q", "-b", "main")
	t.Setenv("AITK_TOOL", "claude-code")
	initRepo(t, dir)
	write(t, dir, "app.py", "print('ok')\n")
	run(t, dir, "git", "add", "app.py")
	newStarted(t, dir, "App", "--ac", "runs")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	write(t, dir, "app.py", "print('ok')\nBROKEN(\n")
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	expect(t, aitk(t, dir, "close", "--summary", "s", "--knowledge", "none"), 3, "E_DOD_CHECK_STALE")
}

func TestReview11CriteriaEditsKeepOtherText(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	id := newStarted(t, dir, "Invoice detail", "--ac", "first", "--ac", "second")
	f := strings.TrimSpace(run(t, dir, "sh", "-c", "ls docs/ai/tasks/*.md"))
	body := strings.Replace(read(t, dir, f), "- [ ] first\n", "- [ ] first\n  - [ ] sub-step of first\nCriteria agreed with PM on Monday.\n", 1)
	write(t, dir, f, body)
	mustOK(t, aitk(t, dir, "task", "update", id, "--ac-done", "2", "--add-ac", "third"))
	got := read(t, dir, f)
	for _, want := range []string{"  - [ ] sub-step of first", "Criteria agreed with PM on Monday.", "- [x] second", "- [ ] third", "- [ ] first"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q after update:\n%s", want, got)
		}
	}
	show := aitk(t, dir, "task", "show", id)
	if n := len(data(show)["criteria"].([]any)); n != 3 {
		t.Fatalf("sub-step counted as a criterion: %d criteria", n)
	}
}

func TestReview12HandWrittenDocsAreNotOverwritten(t *testing.T) {
	dir := repo(t)
	for _, f := range []string{"handoff.md", "knowledge.md", "decisions.md"} {
		write(t, dir, "docs/ai/"+f, "# My own "+f+"\n\nhand-written\n")
	}
	r := aitk(t, dir, "init")
	mustOK(t, r)
	if kept, _ := data(r)["kept_hand_written"].([]any); len(kept) != 3 {
		t.Fatalf("init should report the kept files: %v", data(r))
	}
	newStarted(t, dir, "Anything")
	for _, f := range []string{"handoff.md", "knowledge.md", "decisions.md"} {
		if got := read(t, dir, "docs/ai/"+f); got != "# My own "+f+"\n\nhand-written\n" {
			t.Fatalf("%s overwritten:\n%s", f, got)
		}
	}
	if !strings.Contains(aitk(t, dir, "doctor").stdout, "hand-written") {
		t.Fatal("doctor should mention the hand-written files")
	}
}

// Brief keeps showing project knowledge while files are changed (relevance orders, never hides).
func TestBriefShowsKnowledgeWhileWorking(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	for _, k := range []string{"payments use QRIS", "deploy with deploy.sh", "test accounts are excluded", "plans live in src/data/plans.ts", "notifications go through WAHA"} {
		mustOK(t, aitk(t, dir, "knowledge", "add", k))
	}
	write(t, dir, "web/unrelated-component.tsx", "x\n")
	b := aitk(t, dir, "brief")
	if n := len(data(b)["knowledge"].([]any)); n != 5 {
		t.Fatalf("brief shows %d of 5 knowledge entries while files are changed", n)
	}
}
