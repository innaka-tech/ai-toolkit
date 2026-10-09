package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/innaka-tech/ai-toolkit/internal/schema"
)

// repo creates a git repository with one commit and a Makefile whose test target
// passes when ok.txt exists.
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q", "-b", "main")
	run(t, dir, "git", "config", "user.email", "t@example.com")
	run(t, dir, "git", "config", "user.name", "t")
	write(t, dir, "Makefile", "test:\n\t@test -f ok.txt\n")
	write(t, dir, "README.md", "hello\n")
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "init")
	t.Setenv("AITK_TOOL", "claude-code")
	return dir
}

// initRepo runs aitk init and sets a portable check: passes when ok.txt exists.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	mustOK(t, aitk(t, dir, "init"))
	check := "test -f ok.txt"
	if runtime.GOOS == "windows" {
		check = "if exist ok.txt (exit 0) else (exit 1)"
	}
	cfg := read(t, dir, "aitk.toml")
	cfg = strings.Replace(cfg, `cmd = "make test"`, "cmd = '"+check+"'", 1)
	write(t, dir, "aitk.toml", cfg)
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type res struct {
	code   int
	stdout string
	stderr string
	env    map[string]any
}

// aitk runs the CLI in-process with --json and -C dir, and checks the envelope against its schema.
func aitk(t *testing.T, dir string, args ...string) res {
	t.Helper()
	var out, errb bytes.Buffer
	code := Execute(append([]string{"--json", "-C", dir}, args...), &out, &errb)
	r := res{code: code, stdout: out.String(), stderr: errb.String()}
	if err := json.Unmarshal(out.Bytes(), &r.env); err != nil {
		t.Fatalf("aitk %v: stdout is not one JSON object: %v\n%s", args, err, out.String())
	}
	if err := schema.Validate("result", r.env); err != nil {
		t.Fatalf("aitk %v: envelope invalid: %v", args, err)
	}
	return r
}

func errCode(r res) string {
	if e, ok := r.env["error"].(map[string]any); ok {
		return e["code"].(string)
	}
	return ""
}

func data(r res) map[string]any {
	d, _ := r.env["data"].(map[string]any)
	return d
}

func mustOK(t *testing.T, r res) {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("expected success, got exit %d: %s", r.code, r.stdout)
	}
}

func expect(t *testing.T, r res, exit int, code string) {
	t.Helper()
	if r.code != exit || errCode(r) != code {
		t.Fatalf("expected exit %d %s, got exit %d %s\n%s", exit, code, r.code, errCode(r), r.stdout)
	}
	if e, ok := r.env["error"].(map[string]any); ok && e["fix"] == nil {
		t.Fatalf("error %s has no fix hint", code)
	}
}

// snapshot returns every non-.git file's content, for atomicity checks.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if !info.IsDir() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(dir, p)
			m[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	return m
}

func newStarted(t *testing.T, dir, title string, args ...string) string {
	t.Helper()
	r := aitk(t, dir, append([]string{"task", "new", title, "--start"}, args...)...)
	mustOK(t, r)
	return data(r)["id"].(string)
}

func TestUsageAndNotAProject(t *testing.T) {
	dir := repo(t)
	expect(t, aitk(t, dir, "nope"), 2, "E_USAGE")
	expect(t, aitk(t, dir, "brief"), 3, "E_NOT_A_PROJECT")
	expect(t, aitk(t, t.TempDir(), "init"), 3, "E_NOT_A_PROJECT")
}

func TestInitIdempotentAndPreservesAgents(t *testing.T) {
	dir := repo(t)
	write(t, dir, "AGENTS.md", "# Team rules\n\nUse tabs.\n")
	r := aitk(t, dir, "init")
	mustOK(t, r)
	if data(r)["check"] != "make test" {
		t.Fatalf("check not detected: %v", data(r)["check"])
	}
	agents := read(t, dir, "AGENTS.md")
	if !strings.HasPrefix(agents, "# Team rules\n\nUse tabs.\n") || !strings.Contains(agents, "<!-- aitk:begin v=2 -->") {
		t.Fatalf("AGENTS.md not preserved/extended:\n%s", agents)
	}
	before := snapshot(t, dir)
	mustOK(t, aitk(t, dir, "init"))
	if after := snapshot(t, dir); !equalMaps(before, after) {
		t.Fatal("second init changed files")
	}
	mustOK(t, aitk(t, dir, "doctor"))
}

func TestStandardDefinitionOfDone(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	id := newStarted(t, dir, "Add ok file", "--ac", "Given the repo, when make test runs, then it passes")

	expect(t, aitk(t, dir, "close", "--summary", "s"), 3, "E_DOD_KNOWLEDGE")
	expect(t, aitk(t, dir, "close", "--knowledge", "none"), 3, "E_DOD_SUMMARY")
	expect(t, aitk(t, dir, "close", "--summary", "s", "--knowledge", "none"), 3, "E_DOD_CHECK_STALE")
	expect(t, aitk(t, dir, "check"), 1, "E_CHECK_FAILED")
	expect(t, aitk(t, dir, "close", "--summary", "s", "--knowledge", "none"), 3, "E_DOD_CHECK_STALE")

	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	expect(t, aitk(t, dir, "close", "--summary", "s", "--knowledge", "none"), 3, "E_DOD_ACCEPTANCE")

	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	write(t, dir, "ok.txt", "2") // change after the passing check
	expect(t, aitk(t, dir, "close", "--summary", "s", "--knowledge", "none"), 3, "E_DOD_CHECK_STALE")
	mustOK(t, aitk(t, dir, "check"))

	// A rejected close must not change any file.
	before := snapshot(t, dir)
	expect(t, aitk(t, dir, "close", "--summary", "s", "--knowledge", strings.Repeat("x", 4001)), 3, "E_SCHEMA")
	if !equalMaps(before, snapshot(t, dir)) {
		t.Fatal("rejected close modified files")
	}

	r := aitk(t, dir, "close", "--summary", "Added ok.txt so make test passes", "--knowledge", "make test requires ok.txt", "--tag", "build", "--next", "ship it")
	mustOK(t, r)
	d := data(r)
	if d["status"] != "done" || d["task"] != id || d["knowledge_recorded"] != true {
		t.Fatalf("unexpected close result: %v", d)
	}
	task := read(t, dir, d["handoff"].(string))
	if !strings.Contains(task, "outcome: done") || !strings.Contains(task, "ship it") {
		t.Fatalf("handoff incomplete:\n%s", task)
	}
	if !strings.Contains(read(t, dir, "docs/ai/knowledge/_inbox.md"), "make test requires ok.txt <!-- aitk:k date=") {
		t.Fatal("knowledge not recorded")
	}
	// Session cleared: next close without a task on a non-lite diff must ask for a task.
	if !strings.Contains(read(t, dir, "docs/ai/current-task.md"), "No open tasks") {
		t.Fatal("compat current-task.md not regenerated")
	}
	mustOK(t, aitk(t, dir, "doctor"))
}

func TestLiteImplicitTask(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "aitk init")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	r := aitk(t, dir, "close", "--summary", "Tiny fix", "--knowledge", "none")
	mustOK(t, r)
	if data(r)["profile"] != "lite" || data(r)["status"] != "done" {
		t.Fatalf("expected implicit lite task done: %v", data(r))
	}
}

func TestStrictNeedsRiskAndIndependentReview(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	cfg := read(t, dir, "aitk.toml")
	write(t, dir, "aitk.toml", strings.Replace(cfg, "sensitive_paths = []", `sensitive_paths = ["db/**"]`, 1))
	id := newStarted(t, dir, "Add migration", "--ac", "migration applies")
	write(t, dir, "db/001.sql", "create table x(id int);\n")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	expect(t, aitk(t, dir, "close", "--summary", "s", "--knowledge", "none"), 3, "E_DOD_RISK")

	file := filepath.Join(dir, "docs/ai/tasks")
	entries, _ := os.ReadDir(file)
	rel := "docs/ai/tasks/" + entries[len(entries)-1].Name()
	body := read(t, dir, rel)
	body = strings.Replace(body, "(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)",
		"Impact: new table only.\nSecurity (STRIDE): no new input surface.\nRollback: drop table x.\nValidation: migration test.", 1)
	write(t, dir, rel, body)

	r := aitk(t, dir, "close", "--summary", "s", "--knowledge", "none")
	mustOK(t, r)
	if data(r)["status"] != "in_review" || data(r)["profile"] != "strict" {
		t.Fatalf("strict without review should be in_review: %v", data(r))
	}
	mustOK(t, aitk(t, dir, "task", "start", id))
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "2")) // same tool as implementer
	t.Setenv("AITK_TOOL", "codex")
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	mustOK(t, aitk(t, dir, "check"))
	r = aitk(t, dir, "close", "--summary", "reviewed", "--knowledge", "none")
	mustOK(t, r)
	if data(r)["status"] != "done" {
		t.Fatalf("strict with independent clean review should be done: %v", data(r))
	}
}

func TestBriefBudgetAndDeterminism(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	newStarted(t, dir, "Budgeted task", "--ac", "it works")
	for i := 0; i < 60; i++ {
		mustOK(t, aitk(t, dir, "knowledge", "add", strings.Repeat("Long knowledge about invoices and budgets. ", 8)+string(rune('a'+i%26)), "--tag", "invoice"))
	}
	r1 := aitk(t, dir, "brief", "--budget", "1500")
	mustOK(t, r1)
	if tok := data(r1)["tokens"].(float64); tok > 1500 {
		t.Fatalf("brief over budget: %v tokens", tok)
	}
	if data(r1)["task"] == nil {
		t.Fatal("brief lost the task section")
	}
	r2 := aitk(t, dir, "brief", "--budget", "1500")
	if r1.stdout != r2.stdout {
		t.Fatal("brief is not deterministic")
	}
}

func TestKnowledgeCompactIsLossless(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "docs/ai/knowledge/ops.md", "# Ops\n\nHand-written prose that must survive.\n\n- Deploy via script.\n")
	mustOK(t, aitk(t, dir, "knowledge", "add", "Use decimal strings for money.\nEven in reports.", "--tag", "money"))
	mustOK(t, aitk(t, dir, "knowledge", "add", "use decimal strings for money -- even in reports!", "--tag", "invoice"))
	mustOK(t, aitk(t, dir, "knowledge", "add", "Deploy via script."))
	r := aitk(t, dir, "knowledge", "compact")
	mustOK(t, r)
	if !strings.Contains(read(t, dir, "docs/ai/knowledge/ops.md"), "Hand-written prose that must survive.") {
		t.Fatal("prose was lost")
	}
	money := read(t, dir, "docs/ai/knowledge/money.md")
	if !strings.Contains(money, "Use decimal strings for money.\n  Even in reports.") || !strings.Contains(money, "tags=money,invoice") {
		t.Fatalf("multi-line entry or merged tags missing:\n%s", money)
	}
	if d := data(r); d["duplicates"].(float64) != 2 {
		t.Fatalf("expected 2 duplicates merged, got %v", d["duplicates"])
	}
	mustOK(t, aitk(t, dir, "knowledge", "pin", "decimal strings"))
	money = read(t, dir, "docs/ai/knowledge/money.md")
	if strings.Count(money, "<!-- aitk:k") != 1 || !strings.Contains(money, " pin -->") {
		t.Fatalf("pin broke the multi-line entry:\n%s", money)
	}
	hits := aitk(t, dir, "knowledge", "search", "decimal money")
	if arr, _ := hits.env["data"].([]any); len(arr) == 0 {
		t.Fatal("search found nothing")
	}
}

func TestMigrateFixtureLosslessAndIdempotent(t *testing.T) {
	src, _ := filepath.Abs("../../tests/fixtures/v1-project")
	dir := repo(t)
	copyTree(t, src, dir)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "v1")
	expect(t, aitk(t, dir, "brief"), 3, "E_NEEDS_MIGRATION")

	before := snapshot(t, dir)
	dry := aitk(t, dir, "migrate", "--dry-run")
	mustOK(t, dry)
	if !equalMaps(before, snapshot(t, dir)) {
		t.Fatal("dry run wrote files")
	}
	r := aitk(t, dir, "migrate")
	mustOK(t, r)
	c := data(r)["counts"].(map[string]any)
	for k, v := range map[string]float64{"tasks": 6, "handoffs": 3, "knowledge": 4, "adrs": 2} {
		if c[k] != v {
			t.Fatalf("count %s = %v, want %v", k, c[k], v)
		}
	}
	// Every rewritten v1 file is preserved byte for byte.
	checked := 0
	for rel, content := range before {
		if strings.HasPrefix(rel, "docs/ai/") && rel != "docs/ai/uat-checklist.md" || rel == "ai-state.json" {
			legacy := "docs/ai/_legacy/" + strings.TrimPrefix(rel, "docs/ai/")
			if got := read(t, dir, legacy); got != content {
				t.Fatalf("%s not preserved verbatim", rel)
			}
			checked++
		}
	}
	if checked < 8 {
		t.Fatalf("lossless check covered only %d files", checked)
	}
	if read(t, dir, "docs/ai/uat-checklist.md") != before["docs/ai/uat-checklist.md"] {
		t.Fatal("user content was modified")
	}
	tasks := aitk(t, dir, "task", "list", "--all")
	byTitle := map[string]map[string]any{}
	for _, x := range tasks.env["data"].([]any) {
		m := x.(map[string]any)
		byTitle[m["title"].(string)] = m
	}
	if m := byTitle["Investigate DNS restarts"]; m == nil || m["id"] != "T-004" || m["status"] != "done" {
		t.Fatalf("v1-frontmatter task not converted: %v", m)
	}
	if m := byTitle["F5f pass-21 — File security"]; m == nil || m["id"] == "F5f" || m["legacy"].(map[string]any)["v1_id"] != "F5f" {
		t.Fatalf("colliding prefix not given a fresh id with legacy.v1_id: %v", m)
	}
	if strings.Contains(read(t, dir, "aitk.toml"), "/Users/") {
		t.Fatal("absolute path in aitk.toml")
	}
	mustOK(t, aitk(t, dir, "doctor"))
	b := aitk(t, dir, "brief")
	mustOK(t, b)
	again := aitk(t, dir, "migrate")
	mustOK(t, again)
	if data(again)["already_v2"] != true {
		t.Fatal("second migrate is not a no-op")
	}
}

func TestConcurrentWritersDoNotCorrupt(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	var wg sync.WaitGroup
	codes := make([]string, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var out, errb bytes.Buffer
			if c := Execute([]string{"--json", "-C", dir, "knowledge", "add", "entry number " + string(rune('A'+i))}, &out, &errb); c != 0 {
				codes[i] = out.String()
			}
		}(i)
	}
	wg.Wait()
	for i, c := range codes {
		if c != "" {
			t.Errorf("writer %d failed: %s", i, c)
		}
	}
	inbox := read(t, dir, "docs/ai/knowledge/_inbox.md")
	if n := strings.Count(inbox, "<!-- aitk:k"); n != 20 {
		t.Fatalf("expected 20 entries after concurrent adds, got %d", n)
	}
}

func TestUnicodeTitlesAreSafe(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	r := aitk(t, dir, "task", "new", strings.Repeat("Ubah tampilan 💰 laporan laba ", 10))
	mustOK(t, r)
	title := data(r)["title"].(string)
	if !json.Valid([]byte(`"`+title+`"`)) || len([]rune(title)) > 120 {
		t.Fatalf("bad title truncation: %q", title)
	}
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	var files []string
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	for _, f := range files {
		rel, _ := filepath.Rel(src, f)
		b, _ := os.ReadFile(f)
		write(t, dst, rel, string(b))
	}
}

func TestLiteStillRequiresWrittenCriteria(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "aitk init")
	newStarted(t, dir, "Small change", "--ac", "it works")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	expect(t, aitk(t, dir, "close", "--summary", "s", "--knowledge", "none"), 3, "E_DOD_ACCEPTANCE")
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	r := aitk(t, dir, "close", "--summary", "s", "--knowledge", "none")
	mustOK(t, r)
	if data(r)["profile"] != "lite" {
		t.Fatalf("expected lite profile, got %v", data(r)["profile"])
	}
}
