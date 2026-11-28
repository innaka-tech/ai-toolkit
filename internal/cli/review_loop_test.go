package cli

import (
	"fmt"
	"strings"
	"testing"
)

// standardChange touches enough files that the task is not lite.
func standardChange(t *testing.T, dir, tag string) {
	t.Helper()
	for i := 0; i < 5; i++ {
		write(t, dir, fmt.Sprintf("src/%s_%d.go", tag, i), "package src\n")
	}
	write(t, dir, "ok.txt", "1")
}

func status(t *testing.T, dir, id string) string {
	t.Helper()
	return data(aitk(t, dir, "task", "show", id))["task"].(map[string]any)["status"].(string)
}

func TestReviewUntilCleanBeforeDone(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir) // aitk init writes [quality] review = "standard"
	if !strings.Contains(read(t, dir, "aitk.toml"), `review = "standard"`) {
		t.Fatal("aitk init must turn on review until clean for new projects")
	}
	id := data(aitk(t, dir, "task", "new", "Shipment export", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	standardChange(t, dir, "a")
	mustOK(t, aitk(t, dir, "task", "update", id, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))

	r := aitk(t, dir, "close", "--summary", "export", "--knowledge", "none")
	mustOK(t, r)
	if st := status(t, dir, id); st != "in_review" || !strings.Contains(r.stdout, "review until clean") {
		t.Fatalf("a standard task without a clean review must wait in_review with the loop explained, got %s:\n%s", st, r.stdout)
	}
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "2"))
	mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
	if st := status(t, dir, id); st != "in_review" {
		t.Fatalf("a pass with findings must not finish the task, got %s", st)
	}
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
	if st := status(t, dir, id); st != "done" {
		t.Fatalf("a clean last pass must let the task finish, got %s", st)
	}
}

func TestOneTaskAtATime(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	a := data(aitk(t, dir, "task", "new", "First", "--start"))["id"].(string)
	b := data(aitk(t, dir, "task", "new", "Second"))["id"].(string)

	expect(t, aitk(t, dir, "task", "start", b), 3, "E_TASK_ACTIVE")
	expect(t, aitk(t, dir, "task", "new", "Third", "--start"), 3, "E_TASK_ACTIVE")
	if r := aitk(t, dir, "task", "next", "--start"); r.code == 0 && !strings.Contains(r.stdout, a) {
		t.Fatalf("task next --start must resume the active task or refuse, never switch: %s", r.stdout)
	}
	if r := aitk(t, dir, "task", "start", b); !strings.Contains(r.stdout, "aitk task block "+a) {
		t.Fatalf("the refusal must say how to park the active task: %s", r.stdout)
	}

	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "1")) // in_review still blocks a switch
	if st := status(t, dir, a); st != "in_review" {
		t.Fatalf("setup: want in_review, got %s", st)
	}
	expect(t, aitk(t, dir, "task", "start", b), 3, "E_TASK_ACTIVE")

	mustOK(t, aitk(t, dir, "task", "block", a, "--reason", "waiting for the carrier API"))
	mustOK(t, aitk(t, dir, "task", "start", b))

	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), `review = "standard"`, "review = \"standard\"\none_task = false", 1))
	mustOK(t, aitk(t, dir, "task", "start", a))
}

func TestReviewNotConvergingAndBackCompat(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	id := data(aitk(t, dir, "task", "new", "Hard one", "--start"))["id"].(string)
	for _, n := range []string{"2", "2", "3"} {
		mustOK(t, aitk(t, dir, "review", "pass", "--findings", n))
	}
	md, _ := briefMD(t, dir)
	if !strings.Contains(md, "Review is not converging (2, 2, 3") {
		t.Fatalf("passes that stop decreasing must make the brief say to stop and ask:\n%s", md)
	}
	mustOK(t, aitk(t, dir, "task", "block", id, "--reason", "split it"))

	// Projects without the setting keep the v2 behaviour: only strict work needs reviews.
	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), `review = "standard"`, "", 1))
	id2 := data(aitk(t, dir, "task", "new", "Old project task", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	standardChange(t, dir, "b")
	mustOK(t, aitk(t, dir, "task", "update", id2, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "close", "--summary", "x", "--knowledge", "none"))
	if st := status(t, dir, id2); st != "done" {
		t.Fatalf("without [quality] review a standard task must finish as before, got %s", st)
	}
}

func TestReviewFindingsFromTheLoopReview(t *testing.T) {
	// Lite work never needs the loop: a quick close with no active task stays quick. There is no
	// "all" mode (it needed an exemption that agents could abuse).
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), `review = "standard"`, `review = "all"`, 1))
	expect(t, aitk(t, dir, "brief"), 3, "E_SCHEMA")
	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), `review = "all"`, `review = "standard"`, 1))
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	r := aitk(t, dir, "close", "--summary", "typo fix", "--knowledge", "none")
	mustOK(t, r)
	if id, _ := data(r)["task"].(string); id == "" || status(t, dir, id) != "done" {
		t.Fatalf("a quick lite close has no task to keep reviewing; it must be done: %s", r.stdout)
	}

	// Strict work: closed with no review pass yet, the agent still owes its own pass; once only
	// another tool's pass is missing, the worktree is free. Findings to fix lock it again.
	dir = repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), "sensitive_paths = []", `sensitive_paths = ["db/**"]`, 1))
	a := newStarted(t, dir, "Add payments table", "--ac", "migration applies")
	write(t, dir, "db/001.sql", "create table payments(id int);\n")
	write(t, dir, "ok.txt", "1")
	f := fillRisk(t, dir)
	mustOK(t, aitk(t, dir, "security", "checklist"))
	write(t, dir, f, strings.ReplaceAll(read(t, dir, f), "- [ ] V", "- [x] V"))
	b := data(aitk(t, dir, "task", "new", "Next one"))["id"].(string) // after fillRisk, which expects one task file
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "audit"))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "close", "--summary", "payments", "--knowledge", "none"))
	if st := status(t, dir, a); st != "in_review" {
		t.Fatalf("setup: strict task waits in_review, got %s", st)
	}
	expect(t, aitk(t, dir, "task", "start", b), 3, "E_TASK_ACTIVE") // no pass yet: its own agent reviews first
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "2"))
	expect(t, aitk(t, dir, "task", "start", b), 3, "E_TASK_ACTIVE") // findings to fix
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	mustOK(t, aitk(t, dir, "close", "--summary", "payments", "--knowledge", "none")) // closed after its last pass
	if st := status(t, dir, a); st != "in_review" {
		t.Fatalf("setup: strict task waits for another tool, got %s", st)
	}
	mustOK(t, aitk(t, dir, "task", "start", b)) // only another tool's pass is missing

	// Demoting the active task to todo is not a way around one task at a time.
	c := data(aitk(t, dir, "task", "new", "Third"))["id"].(string)
	mustOK(t, aitk(t, dir, "task", "update", b, "--status", "todo"))
	expect(t, aitk(t, dir, "task", "start", c), 3, "E_TASK_ACTIVE")

	// Convergence: 3, 2, 2 did not improve in the last pass.
	dir = repo(t)
	initRepo(t, dir)
	mustOK(t, aitk(t, dir, "task", "new", "Hard", "--start"))
	for _, n := range []string{"3", "2", "2"} {
		mustOK(t, aitk(t, dir, "review", "pass", "--findings", n))
	}
	if md, _ := briefMD(t, dir); !strings.Contains(md, "Review is not converging (3, 2, 2") {
		t.Fatalf("a last pass that does not improve must trigger the stop-and-ask rule:\n%s", md)
	}
}

func TestReviewLoopSecondPassFindings(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)

	// The rule is in the brief from the start, before the diff grows past lite.
	a := data(aitk(t, dir, "task", "new", "Alpha", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	if md, _ := briefMD(t, dir); !strings.Contains(md, "Review until clean") {
		t.Fatalf("a standard task must see the review rule at task start:\n%s", md)
	}

	// A clean review pass without closing does not free the worktree.
	b := data(aitk(t, dir, "task", "new", "Bravo"))["id"].(string)
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	expect(t, aitk(t, dir, "task", "start", b), 3, "E_TASK_ACTIVE")

	// A clean pass counts only for the code it reviewed: a later change needs a new pass.
	standardChange(t, dir, "c")
	mustOK(t, aitk(t, dir, "task", "update", a, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	write(t, dir, "src/c_0.go", "package src\n\nvar changedAfterReview = 1\n")
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "close", "--summary", "alpha", "--knowledge", "none"))
	if st := status(t, dir, a); st != "in_review" {
		t.Fatalf("code changed after the clean pass: the task must wait for a new pass, got %s", st)
	}
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	mustOK(t, aitk(t, dir, "close", "--summary", "alpha", "--knowledge", "none"))
	if st := status(t, dir, a); st != "done" {
		t.Fatalf("a clean pass on the current code finishes the task, got %s", st)
	}

}

func TestUATAcceptSaysWhyTheTaskStillWaits(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", read(t, dir, "aitk.toml")+"\n[uat]\nrequired = \"standard\"\n")
	id := data(aitk(t, dir, "task", "new", "Export", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	standardChange(t, dir, "u")
	mustOK(t, aitk(t, dir, "task", "update", id, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
	asPerson(t)
	r := aitk(t, dir, "uat", "accept", id, "--by", "alice")
	mustOK(t, r)
	if m, _ := data(r)["missing"].(string); !strings.Contains(m, "review until clean") {
		t.Fatalf("acceptance that cannot finish the task must say what is missing: %s", r.stdout)
	}
}

func TestBriefReviewRuleUsesTheProfileCloseWillUse(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), "sensitive_paths = []", `sensitive_paths = ["secure/**"]`, 1))
	id := data(aitk(t, dir, "task", "new", "Feature", "--start"))["id"].(string)
	// A sensitive change saves the task as strict; once it is gone the diff is standard again.
	write(t, dir, "secure/x.txt", "s\n")
	mustOK(t, aitk(t, dir, "close", "--status", "in_progress", "--summary", "wip", "--knowledge", "none"))
	run(t, dir, "rm", "-f", "secure/x.txt")
	standardChange(t, dir, "p")
	if md, _ := briefMD(t, dir); !strings.Contains(md, "Review until clean") {
		t.Fatalf("an unpinned task whose diff is standard must see the review rule close will apply:\n%s", md)
	}
	// Pinned lite: close uses lite, so the loop does not apply.
	mustOK(t, aitk(t, dir, "task", "update", id, "--profile", "lite"))
	if md, _ := briefMD(t, dir); strings.Contains(md, "Review until clean") {
		t.Fatalf("a task pinned lite must not be told to review until clean:\n%s", md)
	}
}

func TestImplicitCloseStaysExemptAndBriefWithoutTask(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", read(t, dir, "aitk.toml")+"\n[uat]\nrequired = \"all\"\n")
	if md, _ := briefMD(t, dir); strings.Contains(md, "Review until clean") || strings.Contains(md, "finish this task") {
		t.Fatalf("with no task there is nothing to review or finish:\n%s", md)
	}
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	r := aitk(t, dir, "close", "--summary", "typo fix", "--knowledge", "none")
	mustOK(t, r)
	id := data(r)["task"].(string)
	if strings.Contains(r.stdout, "review until clean") {
		t.Fatalf("a quick lite close needs no review loop; the note must name the real wait:\n%s", r.stdout)
	}
	asPerson(t)
	acc := aitk(t, dir, "uat", "accept", id, "--by", "alice")
	mustOK(t, acc)
	if st := status(t, dir, id); st != "done" {
		t.Fatalf("after acceptance a quick lite close must be done (lite needs no review loop), got %s: %s", st, acc.stdout)
	}
}

func TestWaitingOnOthersNeedsAClose(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", read(t, dir, "aitk.toml")+"\n[uat]\nrequired = \"standard\"\n")
	a := data(aitk(t, dir, "task", "new", "Export", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	b := data(aitk(t, dir, "task", "new", "Other"))["id"].(string)
	standardChange(t, dir, "w")
	mustOK(t, aitk(t, dir, "task", "update", a, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))

	// A clean pass alone (task in_review, never closed) does not free the worktree.
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	expect(t, aitk(t, dir, "task", "start", b), 3, "E_TASK_ACTIVE")
	// Handing the task to another tool is not closing it.
	aitk(t, dir, "switch", "codex", "--print")
	expect(t, aitk(t, dir, "task", "next", "--start"), 3, "E_TASK_ACTIVE")

	// in_review with findings: task next --start must refuse too (not switch to b).
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "1"))
	expect(t, aitk(t, dir, "task", "next", "--start"), 3, "E_TASK_ACTIVE")

	// Clean pass, then the code changes, then close: the pass is stale, so it does not wait on others.
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	write(t, dir, "src/w_0.go", "package src\n\nvar later = 1\n")
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
	expect(t, aitk(t, dir, "task", "start", b), 3, "E_TASK_ACTIVE")

	// A clean pass on the current code and a close: only the person's acceptance is missing.
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
	if st := status(t, dir, a); st != "in_review" {
		t.Fatalf("setup: waits for UAT, got %s", st)
	}
	mustOK(t, aitk(t, dir, "task", "start", b))
}

func TestBriefSaysDifferentToolOnlyForStrict(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	mustOK(t, aitk(t, dir, "task", "new", "Standard work", "--ac", "Given x, when y, then z", "--start"))
	standardChange(t, dir, "d") // code without tests: the brief lists aitk-qa
	md, _ := briefMD(t, dir)
	if !strings.Contains(md, "## Personas") || strings.Contains(md, "different tool") {
		t.Fatalf("a standard task's personas section must not say a different tool is needed:\n%s", md)
	}
}

func TestLiteTaskWaitingForUATDoesNotHoldTheWorktree(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir) // review = "standard"
	write(t, dir, "aitk.toml", read(t, dir, "aitk.toml")+"\n[uat]\nrequired = \"all\"\n")
	a := data(aitk(t, dir, "task", "new", "Lite one", "--ac", "x", "--start"))["id"].(string)
	b := data(aitk(t, dir, "task", "new", "Other"))["id"].(string)
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "task", "update", a, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "close", "--summary", "lite", "--knowledge", "none"))
	if st := status(t, dir, a); st != "in_review" {
		t.Fatalf("setup: waits for UAT, got %s", st)
	}
	mustOK(t, aitk(t, dir, "task", "start", b)) // lite never needs the loop; only the person is missing
}

func TestStrictOwnPassIsExplainedToInteractiveAgents(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	mustOK(t, aitk(t, dir, "task", "new", "Auth", "--profile", "strict", "--start"))
	if md, _ := briefMD(t, dir); !strings.Contains(md, "Strict work: review your own change") {
		t.Fatalf("with review = standard the brief must explain the own pass on strict work:\n%s", md)
	}
}

func TestStrictCloseNoteAsksForTheOwnPassFirst(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), "sensitive_paths = []", `sensitive_paths = ["db/**"]`, 1))
	newStarted(t, dir, "Add payments table", "--ac", "migration applies")
	write(t, dir, "db/001.sql", "create table payments(id int);\n")
	write(t, dir, "ok.txt", "1")
	f := fillRisk(t, dir)
	mustOK(t, aitk(t, dir, "security", "checklist"))
	write(t, dir, f, strings.ReplaceAll(read(t, dir, f), "- [ ] V", "- [x] V"))
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "audit"))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	r := aitk(t, dir, "close", "--summary", "payments", "--knowledge", "none")
	mustOK(t, r)
	if !strings.Contains(r.stdout, "first review your own change") {
		t.Fatalf("with review = standard, closing strict work without an own pass must ask for it first:\n%s", r.stdout)
	}
}

func TestStrictIndependentPassMustBeClean(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), "sensitive_paths = []", `sensitive_paths = ["db/**"]`, 1)+"\n[uat]\nrequired = \"strict\"\n")
	id := newStarted(t, dir, "Add payments table", "--ac", "migration applies")
	write(t, dir, "db/001.sql", "create table payments(id int);\n")
	write(t, dir, "ok.txt", "1")
	f := fillRisk(t, dir)
	mustOK(t, aitk(t, dir, "security", "checklist"))
	write(t, dir, f, strings.ReplaceAll(read(t, dir, f), "- [ ] V", "- [x] V"))
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "audit"))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	t.Setenv("AITK_TOOL", "codex")
	mustOK(t, aitk(t, dir, "review", "pass", id, "--findings", "2")) // another tool finds problems
	t.Setenv("AITK_TOOL", "claude-code")
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0")) // the implementer reviews its fix
	mustOK(t, aitk(t, dir, "close", "--summary", "payments", "--knowledge", "none"))
	asPerson(t)
	mustOK(t, aitk(t, dir, "uat", "accept", id, "--by", "alice"))
	if st := status(t, dir, id); st == "done" {
		t.Fatal("another tool's findings followed by the implementer's own clean pass must not count as the independent review, not even after acceptance")
	}
}

// A started task with a small (lite) diff still fixes its review's findings before done.
func TestLiteDiffWithFindingsIsNotDone(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	a := data(aitk(t, dir, "task", "new", "Small", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	b := data(aitk(t, dir, "task", "new", "Other"))["id"].(string)
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "task", "update", a, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "2"))
	aitk(t, dir, "close", "--summary", "small", "--knowledge", "none")
	if st := status(t, dir, a); st == "done" {
		t.Fatal("a lite-sized task must not close as done while its last pass has findings")
	}
	if md, _ := briefMD(t, dir); !strings.Contains(md, "Review until clean") {
		t.Fatalf("the brief must state the rule close enforces on a lite task with findings:\n%s", md)
	}
	expect(t, aitk(t, dir, "task", "start", b), 3, "E_TASK_ACTIVE")
	// Fixed and reviewed clean: done, and the worktree is free.
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	mustOK(t, aitk(t, dir, "close", "--summary", "small", "--knowledge", "none"))
	if st := status(t, dir, a); st != "done" {
		t.Fatalf("after a clean pass the lite task closes as done, got %s", st)
	}
	mustOK(t, aitk(t, dir, "task", "start", b))
}

// A person's own aitk close counts as closing: the next task starts while this one waits for
// acceptance. (A person's uat accept/reject is not a close; see TestUATAcceptIsNotAClose.)
func TestPersonsCloseFreesTheWorktree(t *testing.T) {
	dir := repo(t)
	asPerson(t) // after repo(), which sets claude-code
	initRepo(t, dir)
	write(t, dir, "aitk.toml", read(t, dir, "aitk.toml")+"\n[uat]\nrequired = \"standard\"\n")
	a := data(aitk(t, dir, "task", "new", "Export", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	b := data(aitk(t, dir, "task", "new", "Other"))["id"].(string)
	standardChange(t, dir, "h")
	mustOK(t, aitk(t, dir, "task", "update", a, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
	if st := status(t, dir, a); st != "in_review" {
		t.Fatalf("setup: waits for UAT, got %s", st)
	}
	mustOK(t, aitk(t, dir, "task", "start", b))
}

// A person's uat accept is not a close: strict work whose agent never closed still holds the
// worktree when acceptance comes before the independent pass.
func TestUATAcceptIsNotAClose(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), "sensitive_paths = []", `sensitive_paths = ["db/**"]`, 1)+"\n[uat]\nrequired = \"strict\"\n")
	id := newStarted(t, dir, "Add payments table", "--ac", "migration applies")
	write(t, dir, "db/001.sql", "create table payments(id int);\n")
	write(t, dir, "ok.txt", "1")
	f := fillRisk(t, dir)
	mustOK(t, aitk(t, dir, "security", "checklist"))
	write(t, dir, f, strings.ReplaceAll(read(t, dir, f), "- [ ] V", "- [x] V"))
	other := data(aitk(t, dir, "task", "new", "Other"))["id"].(string)
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "audit"))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0")) // the agent's own pass; it never closes
	asPerson(t)
	mustOK(t, aitk(t, dir, "uat", "accept", id, "--by", "alice"))
	t.Setenv("AITK_TOOL", "claude-code")
	if st := status(t, dir, id); st != "in_review" {
		t.Fatalf("setup: the independent pass is still missing, got %s", st)
	}
	expect(t, aitk(t, dir, "task", "start", other), 3, "E_TASK_ACTIVE")
}

// A task closed to wait for a person stays out of the way when code it did not write arrives
// (a pull, a merge): the close already applied the Definition of Done. Legacy projects too.
func TestUpstreamChangeDoesNotTakeTheWorktreeBack(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		dir := repo(t)
		initRepo(t, dir)
		cfg := read(t, dir, "aitk.toml") + "\n[uat]\nrequired = \"standard\"\n"
		if legacy {
			cfg = strings.Replace(cfg, `review = "standard"`, "", 1)
		}
		write(t, dir, "aitk.toml", cfg)
		a := data(aitk(t, dir, "task", "new", "Export", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
		standardChange(t, dir, "p")
		mustOK(t, aitk(t, dir, "task", "update", a, "--ac-done", "1"))
		mustOK(t, aitk(t, dir, "check"))
		if !legacy {
			mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
		}
		mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
		if st := status(t, dir, a); st != "in_review" {
			t.Fatalf("legacy=%v: setup: waits for UAT, got %s", legacy, st)
		}
		b := data(aitk(t, dir, "task", "new", "Other"))["id"].(string)
		run(t, dir, "git", "add", "-A")
		run(t, dir, "git", "commit", "-qm", "feat: export")
		write(t, dir, "upstream.go", "package main\n") // stands in for a git pull
		run(t, dir, "git", "add", "upstream.go")
		run(t, dir, "git", "commit", "-qm", "feat: upstream")
		mustOK(t, aitk(t, dir, "task", "start", b))
	}
}

// Edits after the close never let unreviewed code reach done: the worktree is free (the close is
// trusted), but acceptance re-checks the Definition of Done on the code as it is.
func TestEditsAfterTheCloseAreCaughtAtAcceptance(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", read(t, dir, "aitk.toml")+"\n[uat]\nrequired = \"standard\"\n")
	a := data(aitk(t, dir, "task", "new", "Export", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	standardChange(t, dir, "e")
	mustOK(t, aitk(t, dir, "task", "update", a, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
	b := data(aitk(t, dir, "task", "new", "Other"))["id"].(string)
	write(t, dir, "src/e_9.go", "package src\n\nvar later = 1\n") // unreviewed
	mustOK(t, aitk(t, dir, "task", "start", b))
	asPerson(t)
	aitk(t, dir, "uat", "accept", a, "--by", "alice")
	if st := status(t, dir, a); st == "done" {
		t.Fatal("acceptance must not finish a task whose code changed after its check and clean pass")
	}
}

// Without a check command, a commit from elsewhere does not take the worktree back either.
func TestUpstreamChangeWithoutACheckCommand(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	cfg := read(t, dir, "aitk.toml")
	cfg = cfg[:strings.Index(cfg, "\n[check]")] + "\n[uat]\nrequired = \"standard\"\n"
	write(t, dir, "aitk.toml", cfg)
	a := data(aitk(t, dir, "task", "new", "Export", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	standardChange(t, dir, "n")
	mustOK(t, aitk(t, dir, "task", "update", a, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
	if st := status(t, dir, a); st != "in_review" {
		t.Fatalf("setup: waits for UAT, got %s", st)
	}
	b := data(aitk(t, dir, "task", "new", "Other"))["id"].(string)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "feat: export")
	write(t, dir, "upstream.go", "package main\n")
	run(t, dir, "git", "add", "upstream.go")
	run(t, dir, "git", "commit", "-qm", "feat: upstream")
	mustOK(t, aitk(t, dir, "task", "start", b))
}

// A close that left the review loop unfinished (the clean pass reviewed older code) keeps the
// worktree even once everything is committed.
func TestUnfinishedLoopHoldsAfterCommitting(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", read(t, dir, "aitk.toml")+"\n[uat]\nrequired = \"standard\"\n")
	a := data(aitk(t, dir, "task", "new", "Export", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	standardChange(t, dir, "c")
	mustOK(t, aitk(t, dir, "task", "update", a, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	write(t, dir, "src/c_9.go", "package src\n\nvar later = 1\n")
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
	b := data(aitk(t, dir, "task", "new", "Other"))["id"].(string)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "feat: export")
	expect(t, aitk(t, dir, "task", "start", b), 3, "E_TASK_ACTIVE")
}

// A pull that grows the diff (commits after the task's base) does not turn a waiting lite task
// into standard: the profile the close saved stands. A stray untracked file changes nothing.
func TestPullDoesNotRaiseTheProfileOfAWaitingTask(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", read(t, dir, "aitk.toml")+"\n[uat]\nrequired = \"all\"\n")
	a := data(aitk(t, dir, "task", "new", "Small", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "task", "update", a, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "close", "--summary", "small", "--knowledge", "none"))
	if st := status(t, dir, a); st != "in_review" {
		t.Fatalf("setup: waits for UAT, got %s", st)
	}
	b := data(aitk(t, dir, "task", "new", "Other"))["id"].(string)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "feat: small")
	for i := 0; i < 12; i++ { // stands in for a pull of a large change
		write(t, dir, fmt.Sprintf("up/u%d.go", i), "package up\n"+strings.Repeat("var _ = 1\n", 30))
	}
	run(t, dir, "git", "add", "up")
	run(t, dir, "git", "commit", "-qm", "feat: upstream")
	write(t, dir, "notes.txt", "stray\n") // untracked
	mustOK(t, aitk(t, dir, "task", "start", b))
}
