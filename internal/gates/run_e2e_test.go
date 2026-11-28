package gates_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
)

type runTaskOut = struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Attempts int    `json:"attempts"`
	Handoff  string `json:"handoff"`
	Commit   string `json:"commit"`
	Stash    string `json:"stash"`
	Note     string `json:"note"`
}

type runOut struct {
	OK   bool `json:"ok"`
	Data struct {
		Planned []struct {
			ID string `json:"id"`
		} `json:"planned"`
		Tasks   []runTaskOut `json:"tasks"`
		Stopped string       `json:"stopped"`
	} `json:"data"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func aitkRun(t *testing.T, dir string, args ...string) runOut {
	t.Helper()
	out, _ := sh(t, dir, bin, append([]string{"--json", "run"}, args...)...)
	var r runOut
	i := strings.Index(out, "{\n")
	if i < 0 || json.Unmarshal([]byte(out[i:]), &r) != nil {
		t.Fatalf("aitk run output is not JSON:\n%s", out)
	}
	return r
}

func runProject(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake agents are shell scripts")
	}
	dir := project(t)
	must(t, dir, bin, "hooks", "uninstall")
	must(t, dir, bin, "task", "new", "First feature", "--ac", "first works")
	must(t, dir, bin, "task", "new", "Second feature", "--ac", "second works")
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: tasks")
	return dir
}

// A well-behaved agent: does the work, passes the check, and closes through the Definition of Done.
const goodAgent = `set -e
echo "$AITK_RUN_TASK" >> worked.txt
touch ok.txt
aitk task update --ac-done 1
aitk check
aitk close --summary "did $AITK_RUN_TASK" --knowledge none
`

func TestRunFinishesTasksThroughTheDefinitionOfDone(t *testing.T) {
	dir := runProject(t)
	dry := aitkRun(t, dir, "--cmd", goodAgent, "--dry-run")
	if len(dry.Data.Planned) != 2 {
		t.Fatalf("dry run must plan both tasks: %+v", dry)
	}
	if _, err := os.Stat(filepath.Join(dir, "worked.txt")); err == nil {
		t.Fatal("dry run must not start the agent")
	}
	r := aitkRun(t, dir, "--cmd", goodAgent, "--commit")
	if !r.OK || len(r.Data.Tasks) != 2 || !strings.HasPrefix(r.Data.Stopped, "no ready tasks") {
		t.Fatalf("both tasks must finish: %+v", r)
	}
	for _, x := range r.Data.Tasks {
		if x.Status != "done" || x.Attempts != 1 || x.Commit == "" {
			t.Fatalf("task not done on the first attempt, or its commit not reported: %+v", x)
		}
	}
	// The first commit holds the first task's work; the second commit has nothing new except state.
	log := must(t, dir, "git", "log", "--format=%s%n%b", "-3")
	if !strings.Contains(log, "feat: first feature") || !strings.Contains(log, "AI-Task: T-") {
		t.Fatalf("one Conventional Commit per finished task expected:\n%s", log)
	}
}

func TestRunHandsOverUnfinishedTasksAndStops(t *testing.T) {
	dir := runProject(t)
	lazy := `echo "$AITK_RUN_TASK" >> worked.txt; cat > /dev/null; exit 0`
	r := aitkRun(t, dir, "--cmd", lazy, "--max-attempts", "2")
	if len(r.Data.Tasks) != 2 || !strings.Contains(r.Data.Stopped, "two tasks in a row") {
		t.Fatalf("run must stop after two unfinished tasks: %+v", r)
	}
	for _, x := range r.Data.Tasks {
		if x.Status != "blocked" || x.Attempts != 2 || x.Handoff == "" {
			t.Fatalf("unfinished task must be handed over as blocked after 2 attempts: %+v", x)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "worked.txt"))
	if n := strings.Count(string(b), "\n"); n != 4 {
		t.Fatalf("agent should have run 4 times, ran %d", n)
	}
	// The prompt reached the agent on stdin and in the prompt file.
	echo := `cat "$AITK_RUN_PROMPT_FILE" > prompt-file.txt; cat > prompt-stdin.txt`
	must(t, dir, bin, "task", "new", "Third feature", "--ac", "third works")
	aitkRun(t, dir, "--cmd", echo, "--max-attempts", "1", "--max-tasks", "1")
	for _, f := range []string{"prompt-file.txt", "prompt-stdin.txt"} {
		b, _ := os.ReadFile(filepath.Join(dir, f))
		if !strings.Contains(string(b), "Third feature") || !strings.Contains(string(b), "aitk close") {
			t.Fatalf("%s lacks the task or the rules:\n%s", f, b)
		}
	}
}

func TestRunRefusesNestedRuns(t *testing.T) {
	dir := runProject(t)
	nested := `aitk --json run --cmd true > nested.json; exit 0`
	aitkRun(t, dir, "--cmd", nested, "--max-tasks", "1", "--max-attempts", "1")
	b, _ := os.ReadFile(filepath.Join(dir, "nested.json"))
	if !strings.Contains(string(b), "E_RUN_NESTED") {
		t.Fatalf("an agent started by aitk run must not start another run:\n%s", b)
	}
}

func TestRunStopsWhenTheAgentCannotStart(t *testing.T) {
	dir := runProject(t)
	r := aitkRun(t, dir, "--cmd", "echo 'error: unexpected argument' >&2; exit 2")
	if len(r.Data.Tasks) != 1 || !strings.Contains(r.Data.Stopped, "failed to start") {
		t.Fatalf("a broken agent must stop the run at once: %+v", r)
	}
	x := r.Data.Tasks[0]
	if x.Status != "in_progress" || x.Handoff != "" || !strings.Contains(x.Note, "unexpected argument") {
		t.Fatalf("the task must not be blamed for a broken agent: %+v", x)
	}
	// After fixing the agent, the run resumes with the same task.
	r = aitkRun(t, dir, "--cmd", goodAgent, "--max-tasks", "1")
	if len(r.Data.Tasks) != 1 || r.Data.Tasks[0].ID != x.ID || r.Data.Tasks[0].Status != "done" {
		t.Fatalf("the interrupted task must be resumed first: %+v", r)
	}
}

// Review A #1: a status edited by hand is not done.
func TestRunRejectsForgedDone(t *testing.T) {
	dir := runProject(t)
	forge := `f=$(ls docs/ai/tasks/*.md | xargs grep -l "$AITK_RUN_TASK" | head -1); sed -i.bak 's/^status: in_progress/status: done/' "$f"; rm -f "$f.bak"; touch ok.txt`
	r := aitkRun(t, dir, "--cmd", forge, "--max-tasks", "1", "--commit")
	if r.Error != nil {
		t.Fatalf("run failed: %+v", r)
	}
	x := r.Data.Tasks[0]
	if x.Status != "blocked" || !strings.Contains(x.Note, "verification failed") || x.Attempts != 2 {
		t.Fatalf("a forged done must be reopened and handed over: %+v", x)
	}
	if log := must(t, dir, "git", "log", "--format=%s", "-1"); strings.HasPrefix(log, "feat:") {
		t.Fatalf("forged work must not be committed as a feature: %s", log)
	}
	// A run agent cannot accept UAT for a person either.
	out, err := sh(t, dir, "sh", "-c", "AITK_RUN=1 AITK_TOOL= "+bin+" uat accept "+x.ID)
	if err == nil || !strings.Contains(out, "E_UAT_AGENT") {
		t.Fatalf("uat accept must refuse an agent started by aitk run:\n%s", out)
	}
}

// Review A #2: --commit needs a clean tree, and unfinished work never reaches a commit.
func TestRunCommitHoldsOnlyTheTasksWork(t *testing.T) {
	dir := runProject(t)
	write(t, dir, "local-notes.txt", "DB_PASSWORD=hunter2\n")
	r := aitkRun(t, dir, "--cmd", goodAgent, "--commit")
	if r.Error == nil || r.Error.Code != "E_RUN_DIRTY" {
		t.Fatalf("--commit must refuse a dirty tree: %+v", r)
	}
	os.Remove(filepath.Join(dir, "local-notes.txt"))
	// First task: the agent leaves partial work and gives up; second task: done.
	mark := filepath.Join(t.TempDir(), "first.done") // outside the repository: stashing must not reset it
	agent := "if [ ! -f " + mark + " ]; then touch " + mark + " partial.txt; aitk close --status blocked --summary \"cannot finish\" --knowledge none; else\n" + goodAgent + "fi\n"
	r = aitkRun(t, dir, "--cmd", agent, "--commit")
	if len(r.Data.Tasks) != 2 || r.Data.Tasks[0].Status != "blocked" || r.Data.Tasks[0].Stash == "" || r.Data.Tasks[1].Status != "done" {
		t.Fatalf("unexpected run: %+v", r)
	}
	files := must(t, dir, "git", "show", "--name-only", "--format=", "HEAD")
	if strings.Contains(files, "partial.txt") {
		t.Fatalf("the second task's commit holds the first task's leftovers:\n%s", files)
	}
	if stash := must(t, dir, "git", "stash", "list"); !strings.Contains(stash, "unfinished work on") {
		t.Fatalf("unfinished work must be stashed, not lost:\n%s", stash)
	}
	if st := strings.TrimSpace(must(t, dir, "git", "status", "--porcelain")); st != "" {
		t.Fatalf("the run must leave a clean tree:\n%s", st)
	}
}

// Review A #3: a timeout ends everything the agent started.
func TestRunTimeoutKillsTheAgentsChildren(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for a one-minute timeout")
	}
	dir := runProject(t)
	agent := `(sleep 66; touch late.txt) & sleep 300`
	start := time.Now()
	r := aitkRun(t, dir, "--cmd", agent, "--timeout", "1", "--max-attempts", "1", "--max-tasks", "1")
	_ = r
	if time.Since(start) > 100*time.Second {
		t.Fatal("the run waited for the agent beyond its timeout")
	}
	time.Sleep(time.Until(start.Add(70 * time.Second)))
	if _, err := os.Stat(filepath.Join(dir, "late.txt")); err == nil {
		t.Fatal("a child of the timed-out agent kept running and wrote into the repository")
	}
}

// Review A #7: one run per worktree.
func TestRunLock(t *testing.T) {
	dir := runProject(t)
	agent := bin + ` --json run --cmd true > second-run.json; ` + goodAgent
	aitkRun(t, dir, "--cmd", agent, "--max-tasks", "1")
	b, _ := os.ReadFile(filepath.Join(dir, "second-run.json"))
	if !strings.Contains(string(b), "E_RUN_NESTED") && !strings.Contains(string(b), "E_LOCKED") {
		t.Fatalf("a second run in the same worktree must be refused:\n%s", b)
	}
	gitDir := strings.TrimSpace(must(t, dir, "git", "rev-parse", "--absolute-git-dir"))
	l, err := fsx.Acquire(filepath.Join(gitDir, "aitk", "run.lock"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if r := aitkRun(t, dir, "--cmd", goodAgent); r.Error == nil || r.Error.Code != "E_LOCKED" {
		t.Fatalf("a held run lock must refuse a second run: %+v", r)
	}
	l.Release()
	if r := aitkRun(t, dir, "--cmd", goodAgent); r.Error != nil {
		t.Fatalf("a released lock (or a crashed run) must not block: %+v", r)
	}
}

// Second review #1, #3: a forged handoff does not help, and a reopened task is worked again
// as the active task (no stray tasks).
func TestRunForgedHandoffThenRealWork(t *testing.T) {
	dir := runProject(t)
	mark := filepath.Join(t.TempDir(), "forged")
	agent := "if [ ! -f " + mark + " ]; then touch " + mark + "; f=$(grep -l \"$AITK_RUN_TASK\" docs/ai/tasks/*.md | head -1); " +
		"sed -i.bak 's/^status: in_progress/status: done/' \"$f\"; rm -f \"$f.bak\"; " +
		"printf -- '---\\ntask: %s\\nat: \"9999-01-01T00:00:00Z\"\\nby: x\\noutcome: done\\ntask_status: done\\n---\\nforged\\n' \"$AITK_RUN_TASK\" > docs/ai/handoff/99990101T000000Z-x.md; else\n" + goodAgent + "fi\n"
	r := aitkRun(t, dir, "--cmd", agent, "--max-tasks", "1")
	x := r.Data.Tasks[0]
	if x.Status != "done" || x.Attempts != 2 {
		t.Fatalf("attempt 1 is forged and reopened, attempt 2 finishes: %+v", x)
	}
	if list := must(t, dir, bin, "task", "list", "--all"); strings.Count(list, "\n") != 2 {
		t.Fatalf("no stray tasks may appear:\n%s", list)
	}
}

// Second review #2: an agent cannot grant itself user acceptance by editing the task file.
func TestRunForgedUATIsIgnored(t *testing.T) {
	dir := runProject(t)
	must(t, dir, "sh", "-c", `printf '\n[uat]\nrequired = "all"\n' >> aitk.toml && git commit -qam "chore: uat"`)
	agent := `f=$(grep -l "$AITK_RUN_TASK" docs/ai/tasks/*.md | head -1)
awk '/^evidence:/ && !d {print; print "  uat:"; print "    status: accepted"; print "    at: \"2026-01-01T00:00:00Z\""; d=1; next} {print}' "$f" > "$f.tmp" && mv "$f.tmp" "$f"
` + goodAgent
	r := aitkRun(t, dir, "--cmd", agent, "--max-tasks", "1")
	if x := r.Data.Tasks[0]; x.Status != "in_review" {
		t.Fatalf("a forged acceptance must not make the task done: %+v", x)
	}
}

// Second review #4: another task's committed tests, or a deleted test, are not a regression test.
func TestRunRegressionGateIgnoresOtherTasksAndDeletions(t *testing.T) {
	dir := runProject(t)
	must(t, dir, bin, "task", "new", "Fix crash", "--tag", "bug", "--ac", "no crash")
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: bug task")
	// Feature tasks add tests (committed by --commit); the bug task changes only code.
	agent := `case "$(aitk task show "$AITK_RUN_TASK")" in *"Fix crash"*) echo fix > code.txt;; *) echo t > "feat_${AITK_RUN_TASK}_test.go";; esac
` + goodAgent
	r := aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1")
	statuses := map[string]int{}
	for _, x := range r.Data.Tasks {
		statuses[x.Status]++
	}
	if statuses["done"] != 2 || statuses["blocked"] != 1 {
		t.Fatalf("the bug task must not pass on the feature tasks' tests: %+v", r.Data.Tasks)
	}
	if h := must(t, dir, "sh", "-c", "cat docs/ai/handoff/*.md"); !strings.Contains(h, "agent exited with 3") {
		t.Fatalf("the bug task's close must have been refused:\n%s", h)
	}
	// Deleting a test is not a regression test either.
	must(t, dir, bin, "task", "update", "--status", "todo", "--", firstBug(t, dir))
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: reopen")
	del := `git rm -q feat_*_test.go; echo fix > code.txt
` + goodAgent
	r = aitkRun(t, dir, "--cmd", del, "--max-attempts", "1", "--max-tasks", "1")
	if x := r.Data.Tasks[0]; x.Status == "done" {
		t.Fatalf("deleting tests must not satisfy the gate: %+v", x)
	}
}

func firstBug(t *testing.T, dir string) string {
	t.Helper()
	for _, l := range strings.Split(must(t, dir, bin, "task", "list", "--all"), "\n") {
		if strings.Contains(l, "Fix crash") {
			return strings.Fields(strings.TrimPrefix(strings.TrimSpace(l), "*"))[0]
		}
	}
	t.Fatal("bug task not found")
	return ""
}

// Second review #5: a rename in unfinished work is stashed as a whole; history stays clean.
func TestRunStashKeepsRenamesTogether(t *testing.T) {
	dir := runProject(t)
	write(t, dir, "old.txt", "keep\n")
	write(t, dir, ":weird.txt", "x\n")
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: files")
	agent := `git mv old.txt new.txt; echo y >> ./:weird.txt; aitk close --status blocked --summary "stuck" --knowledge none`
	r := aitkRun(t, dir, "--cmd", agent, "--commit", "--max-tasks", "1")
	if r.Error != nil || r.Data.Tasks[0].Stash == "" {
		t.Fatalf("unfinished work must be stashed: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.txt")); err != nil {
		t.Fatal("the rename must be undone in the working tree")
	}
	if files := must(t, dir, "git", "show", "--name-status", "--format=", "HEAD"); strings.Contains(files, "old.txt") || strings.Contains(files, "weird") {
		t.Fatalf("the handover commit must hold only aitk's records:\n%s", files)
	}
	if st := strings.TrimSpace(must(t, dir, "git", "status", "--porcelain")); st != "" {
		t.Fatalf("tree must be clean:\n%s", st)
	}
}

// Third review A: an agent cannot lower its own bar (profile, pin, tags, criteria).
func TestRunAgentCannotLowerItsOwnBar(t *testing.T) {
	dir := runProject(t)
	must(t, dir, "sh", "-c", `printf '\n[uat]\nrequired = "standard"\n' >> aitk.toml && git commit -qam "chore: uat"`)
	must(t, dir, bin, "task", "new", "Fix crash", "--tag", "bug", "--tag", "target", "--ac", "no crash")
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: bug task")
	// Does everything right except a test, then makes the task lite, pinned, untagged, and done.
	lower := `aitk task update --ac-done 1; touch ok.txt; echo fix > code.txt; aitk check
f=$(grep -l "$AITK_RUN_TASK" docs/ai/tasks/*.md | head -1)
awk '/^profile:/{print "profile: lite"; print "profile_pinned: true"; next} /^  - bug$/{next} /^status:/{print "status: done"; next} {print}' "$f" > "$f.tmp" && mv "$f.tmp" "$f"`
	r := aitkRun(t, dir, "--cmd", lower, "--max-attempts", "1", "--tag", "target")
	if len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Status == "done" {
		t.Fatalf("lowering the profile or dropping the bug tag must not finish a task: %+v", r.Data.Tasks)
	}
	must(t, dir, bin, "task", "new", "Reword target", "--tag", "reword", "--ac", "the real requirement")
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	reword := `f=$(grep -l "$AITK_RUN_TASK" docs/ai/tasks/*.md | head -1)
sed -i.bak -e 's/- \[ \] the real requirement/- [ ] nothing to do/' "$f"; rm -f "$f.bak"
` + goodAgent
	r = aitkRun(t, dir, "--cmd", reword, "--max-attempts", "1", "--tag", "reword")
	if len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Status == "done" || !strings.Contains(r.Data.Tasks[0].Note, "reworded") {
		t.Fatalf("rewording a criterion must not finish a task: %+v", r.Data.Tasks)
	}
}

// Codex review: a forged acceptance does not survive an attempt that ends without close.
func TestRunForgedUATDoesNotSurviveHandover(t *testing.T) {
	dir := runProject(t)
	forge := `f=$(grep -l "$AITK_RUN_TASK" docs/ai/tasks/*.md | head -1)
awk '/^updated:/ {print; print "evidence:"; print "  uat:"; print "    status: accepted"; print "    at: \"2026-01-01T00:00:00Z\""; next} {print}' "$f" > "$f.tmp" && mv "$f.tmp" "$f"`
	r := aitkRun(t, dir, "--cmd", forge, "--max-attempts", "1", "--max-tasks", "1")
	x := r.Data.Tasks[0]
	if x.Status != "blocked" {
		t.Fatalf("unexpected: %+v", x)
	}
	if body := must(t, dir, "sh", "-c", "grep -l "+x.ID+" docs/ai/tasks/*.md | xargs cat"); strings.Contains(body, "status: accepted") {
		t.Fatalf("the forged acceptance survived the handover:\n%s", body)
	}
}

// A standard-sized task in a new project (review = "standard"): the agent reviews its own work
// until a pass finds 0, and aitk run counts that loop, so dependent tasks are not left waiting.
const reviewingAgent = `set -e
for i in 1 2 3 4 5 6; do echo "$AITK_RUN_TASK" > "src_${AITK_RUN_TASK}_$i.txt"; done
touch ok.txt
aitk task update --ac-done 1
aitk check
aitk review pass --findings 1
aitk review pass --findings 0
aitk close --summary "did $AITK_RUN_TASK" --knowledge none
`

func TestRunCountsTheAgentsReviewLoopOnStandardTasks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake agents are shell scripts")
	}
	dir := project(t)
	if b, _ := os.ReadFile(filepath.Join(dir, "aitk.toml")); !strings.Contains(string(b), `review = "standard"`) {
		t.Fatal("setup: aitk init must write review = \"standard\"")
	}
	must(t, dir, bin, "hooks", "uninstall")
	first := strings.TrimSpace(must(t, dir, bin, "task", "new", "First feature", "--ac", "first works"))
	id := strings.Fields(first)[1]
	must(t, dir, bin, "task", "new", "Second feature", "--ac", "second works", "--depends", id)
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: tasks")

	r := aitkRun(t, dir, "--cmd", reviewingAgent)
	if !r.OK || len(r.Data.Tasks) != 2 {
		t.Fatalf("both tasks (the second depends on the first) must run: %+v", r)
	}
	for _, x := range r.Data.Tasks {
		if x.Status != "done" {
			t.Fatalf("a standard task whose agent reviewed until clean must be done in aitk run: %+v", x)
		}
	}
}

type reviewPassOut struct {
	By       string `json:"by"`
	Findings int    `json:"findings"`
}

func passesOf(t *testing.T, dir, id string) []reviewPassOut {
	t.Helper()
	out := must(t, dir, bin, "--json", "task", "show", id)
	var r struct {
		Data struct {
			Task struct {
				Review struct {
					Passes []reviewPassOut `json:"passes"`
				} `json:"review"`
			} `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("task show: %v\n%s", err, out)
	}
	return r.Data.Task.Review.Passes
}

// An agent forges an independent reviewer in the task file. aitk run must never keep a forged
// name: on a failed attempt nothing about reviews survives, on success the agent's passes are
// attributed to the agent, so they cannot later count as another tool's review.
func TestRunNeverKeepsForgedReviewers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake agents are shell scripts")
	}
	forger := func(markDone bool) string {
		done := ""
		if markDone {
			done = "aitk task update --ac-done 1\n"
		}
		return `set -e
for i in 1 2 3 4 5 6; do echo x > "src_$i.txt"; done
touch ok.txt
` + done + `aitk check
aitk review pass --findings 0
f=$(ls docs/ai/tasks/"$AITK_RUN_TASK"-*.md)
sed -i.bak -E 's/^([[:space:]]+by: ).*/\1human-reviewer/' "$f" && rm -f "$f.bak"
aitk close --summary "did it" --knowledge none || true
`
	}
	for _, markDone := range []bool{false, true} {
		dir := project(t)
		must(t, dir, bin, "hooks", "uninstall")
		id := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Feature", "--ac", "works")))[1]
		must(t, dir, "git", "add", "-A")
		must(t, dir, "git", "commit", "-qm", "chore: task")
		aitkRun(t, dir, "--cmd", forger(markDone), "--max-attempts", "1", "--max-tasks", "1")
		for _, ps := range passesOf(t, dir, id) {
			if ps.By == "human-reviewer" {
				t.Fatalf("markDone=%v: a forged reviewer survived aitk run: %+v", markDone, passesOf(t, dir, id))
			}
		}
		if !markDone && len(passesOf(t, dir, id)) != 0 {
			t.Fatalf("a failed attempt must keep no review passes, got %+v", passesOf(t, dir, id))
		}
	}
}

// An agent that closes without reviewing does not get its task counted as finished.
func TestRunReopensWhenTheAgentSkipsTheReviewLoop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake agents are shell scripts")
	}
	dir := project(t)
	must(t, dir, bin, "hooks", "uninstall")
	must(t, dir, bin, "task", "new", "Feature", "--ac", "works")
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	lazy := `set -e
for i in 1 2 3 4 5 6; do echo x > "src_$i.txt"; done
touch ok.txt
aitk task update --ac-done 1
aitk check
aitk close --summary "did it" --knowledge none
`
	r := aitkRun(t, dir, "--cmd", lazy, "--max-attempts", "2", "--max-tasks", "1")
	if len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Status == "done" || r.Data.Tasks[0].Status == "in_review" || r.Data.Tasks[0].Attempts != 2 {
		t.Fatalf("skipping the review loop must use the remaining attempts and not count as finished: %+v", r.Data.Tasks)
	}
	if grep := must(t, dir, "sh", "-c", "cat docs/ai/handoff/*.md"); !strings.Contains(grep, "review until clean") {
		t.Fatalf("the handover must say the review loop was not finished:\n%s", grep)
	}
}

// Run 1 finishes a standard task with the agent's own review loop (its passes are kept under
// the agent's name). The task is reopened; run 2 makes it strict and removes the agent from the
// workers. The agent's old passes must never count as the independent review.
func TestRunAgentCannotCertifyStrictReviewByFlippingTheProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake agents are shell scripts")
	}
	dir := project(t)
	must(t, dir, bin, "hooks", "uninstall")
	cfg, _ := os.ReadFile(filepath.Join(dir, "aitk.toml"))
	c := strings.Replace(string(cfg), "sensitive_paths = []", `sensitive_paths = ["secure/**"]`, 1) + "\n[security]\naudit = \"off\"\nasvs_level = 0\n"
	if err := os.WriteFile(filepath.Join(dir, "aitk.toml"), []byte(c), 0o644); err != nil {
		t.Fatal(err)
	}
	id := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Feature", "--ac", "works")))[1]
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	if r := aitkRun(t, dir, "--cmd", reviewingAgent, "--max-tasks", "1"); len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Status != "done" {
		t.Fatalf("setup: run 1 must finish the standard task: %+v", r.Data.Tasks)
	}
	if ps := passesOf(t, dir, id); len(ps) == 0 {
		t.Fatal("setup: run 1 must keep the agent's passes")
	}
	must(t, dir, bin, "task", "update", id, "--status", "todo") // reopened by a person
	flip := `set -e
f=$(ls docs/ai/tasks/"$AITK_RUN_TASK"-*.md)
mkdir -p secure && echo s > secure/x.txt
sed -i.bak 's/(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)/Impact: x. Security (STRIDE): y. Rollback: z. Validation: w./' "$f"
aitk check
aitk close --summary "second" --knowledge none || true
sed -i.bak -E '/^workers:/,/^[a-z_]+:/{/^[[:space:]]+- /d;}' "$f"
sed -i.bak -E 's/^created_by: .*/created_by: human/' "$f"
sed -i.bak -E 's/^status: .*/status: done/' "$f"
rm -f "$f.bak"
`
	aitkRun(t, dir, "--cmd", flip, "--max-attempts", "1", "--max-tasks", "1")
	var r struct {
		Data struct {
			Task struct {
				Status  string `json:"status"`
				Profile string `json:"profile"`
			} `json:"task"`
		} `json:"data"`
	}
	json.Unmarshal([]byte(must(t, dir, bin, "--json", "task", "show", id)), &r)
	if r.Data.Task.Status == "done" {
		t.Fatalf("strict work finished on the agent's own passes (profile %s): %+v", r.Data.Task.Profile, passesOf(t, dir, id))
	}
}

// The review step is in the run prompt only where the agent's own loop applies, and a task that
// finishes on a later attempt carries no stale note from an earlier one.
func TestRunPromptReviewStepAndNotes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake agents are shell scripts")
	}
	probe := `grep -c "review your own change" "$AITK_RUN_PROMPT_FILE" > prompt-count.txt || true
aitk close --status blocked --summary "probe only" --knowledge none
`
	for _, tc := range []struct {
		review string
		want   string
	}{{`review = "standard"`, "1"}, {"", "0"}} {
		dir := project(t)
		must(t, dir, bin, "hooks", "uninstall")
		cfg, _ := os.ReadFile(filepath.Join(dir, "aitk.toml"))
		os.WriteFile(filepath.Join(dir, "aitk.toml"), []byte(strings.Replace(string(cfg), `review = "standard"`, tc.review, 1)), 0o644)
		must(t, dir, bin, "task", "new", "Feature", "--ac", "works")
		must(t, dir, "git", "add", "-A")
		must(t, dir, "git", "commit", "-qm", "chore: task")
		aitkRun(t, dir, "--cmd", probe, "--max-attempts", "1", "--max-tasks", "1")
		if b, _ := os.ReadFile(filepath.Join(dir, "prompt-count.txt")); strings.TrimSpace(string(b)) != tc.want {
			t.Fatalf("config %q: review step count in prompt = %q, want %s", tc.review, b, tc.want)
		}
	}

	dir := project(t)
	must(t, dir, bin, "hooks", "uninstall")
	must(t, dir, bin, "task", "new", "Feature", "--ac", "works")
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	late := `set -e
for i in 1 2 3 4 5 6; do echo x > "src_$i.txt"; done
touch ok.txt
aitk task update --ac-done 1
aitk check
if [ -f .reviewed-once ]; then aitk review pass --findings 0; fi
touch .reviewed-once
aitk close --summary "did it" --knowledge none
`
	r := aitkRun(t, dir, "--cmd", late, "--max-attempts", "2", "--max-tasks", "1")
	if len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Status != "done" || r.Data.Tasks[0].Attempts != 2 || r.Data.Tasks[0].Note != "" {
		t.Fatalf("a task finished on the second attempt must be done with no stale note: %+v", r.Data.Tasks)
	}
}

func newRunProject(t *testing.T, extraCfg string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake agents are shell scripts")
	}
	dir := project(t)
	must(t, dir, bin, "hooks", "uninstall")
	if extraCfg != "" {
		cfg, _ := os.ReadFile(filepath.Join(dir, "aitk.toml"))
		c := strings.Replace(string(cfg), "sensitive_paths = []", `sensitive_paths = ["secure/**"]`, 1) + extraCfg
		if err := os.WriteFile(filepath.Join(dir, "aitk.toml"), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func statusOf(t *testing.T, dir, id string) string {
	t.Helper()
	var r struct {
		Data struct {
			Task struct {
				Status string `json:"status"`
			} `json:"task"`
		} `json:"data"`
	}
	json.Unmarshal([]byte(must(t, dir, bin, "--json", "task", "show", id)), &r)
	return r.Data.Task.Status
}

// Inside aitk run the agent cannot start another task (it would be finished outside the run's
// verification).
func TestRunAgentStaysOnItsOwnTask(t *testing.T) {
	dir := newRunProject(t, "")
	a := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "First", "--ac", "works")))[1]
	b := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Second", "--ac", "works", "--depends", a)))[1]
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: tasks")
	escape := `aitk task block "$AITK_RUN_TASK" --reason "going elsewhere" || true
aitk --json task start ` + b + ` > escape.json 2>&1 || true
aitk --json task new "Sneaky" --start > sneaky.json 2>&1 || true
`
	aitkRun(t, dir, "--cmd", escape, "--max-attempts", "1", "--max-tasks", "1")
	for _, f := range []string{"escape.json", "sneaky.json"} {
		if out, _ := os.ReadFile(filepath.Join(dir, f)); !strings.Contains(string(out), "E_RUN_SCOPE") {
			t.Fatalf("%s: starting another task inside aitk run must fail with E_RUN_SCOPE:\n%s", f, out)
		}
	}
	if st := statusOf(t, dir, b); st != "todo" {
		t.Fatalf("the other task must stay untouched, got %s", st)
	}
}

// A strict task that aitk run leaves waiting for another tool does not hold the worktree.
func TestRunStrictTaskWaitingForAnotherToolFreesTheWorktree(t *testing.T) {
	dir := newRunProject(t, "\n[security]\naudit = \"off\"\nasvs_level = 0\n")
	id := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Auth", "--ac", "works", "--profile", "strict")))[1]
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	agent := `set -e
f=$(ls docs/ai/tasks/"$AITK_RUN_TASK"-*.md)
mkdir -p secure && echo s > secure/x.txt
sed -i.bak 's/(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)/Impact: x. Security (STRIDE): y. Rollback: z. Validation: w./' "$f" && rm -f "$f.bak"
touch ok.txt
aitk task update --ac-done 1
aitk check
# Follows the prompt only: it reviews its own change because the prompt says so.
if grep -q "review your own change" "$AITK_RUN_PROMPT_FILE"; then aitk review pass --findings 0; fi
aitk close --summary "auth" --knowledge none
`
	r := aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	if len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Status != "in_review" || !strings.Contains(r.Data.Tasks[0].Note, "another tool") {
		t.Fatalf("setup: strict task must wait for another tool: %+v", r.Data.Tasks)
	}
	if ps := passesOf(t, dir, id); len(ps) != 1 || ps[0].By != "custom" {
		t.Fatalf("the agent's own pass is kept under its name: %+v", ps)
	}
	if out, err := sh(t, dir, bin, "task", "new", "Next", "--start"); err != nil {
		t.Fatalf("a task that only waits for another tool must not hold the worktree: %v\n%s", err, out)
	}
}

// Run 1 by one agent finishes a standard task with its own passes; the task is reopened; run 2
// by a different agent (codex) makes it strict and removes run 1's agent from the workers. Run 1's
// passes must still not count as independent.
func TestRunWorkersSurviveAcrossAgents(t *testing.T) {
	dir := newRunProject(t, "\n[security]\naudit = \"off\"\nasvs_level = 0\n")
	id := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Feature", "--ac", "works")))[1]
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	if r := aitkRun(t, dir, "--cmd", reviewingAgent, "--max-tasks", "1"); len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Status != "done" {
		t.Fatalf("setup: run 1 must finish the task: %+v", r.Data.Tasks)
	}
	must(t, dir, bin, "task", "update", id, "--status", "todo")
	fake := t.TempDir()
	script := `#!/bin/sh
set -e
f=$(ls docs/ai/tasks/"$AITK_RUN_TASK"-*.md)
mkdir -p secure && echo s > secure/x.txt
sed -i.bak 's/(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)/Impact: x. Security (STRIDE): y. Rollback: z. Validation: w./' "$f"
aitk check
aitk close --summary "codex" --knowledge none || true
sed -i.bak -E '/^workers:/,/^[a-z_]+:/{/^[[:space:]]+- custom$/d;}' "$f"
sed -i.bak -E 's/^created_by: .*/created_by: human/' "$f"
sed -i.bak -E 's/^status: .*/status: done/' "$f"
rm -f "$f.bak"
`
	if err := os.WriteFile(filepath.Join(fake, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fake+string(os.PathListSeparator)+os.Getenv("PATH"))
	aitkRun(t, dir, "--agent", "codex", "--max-attempts", "1", "--max-tasks", "1")
	if st := statusOf(t, dir, id); st == "done" {
		t.Fatalf("strict work finished on another agent's earlier passes: %+v", passesOf(t, dir, id))
	}
}

// The run scope also covers aitk work: an agent cannot open another task in a new worktree.
func TestRunScopeCoversWork(t *testing.T) {
	dir := newRunProject(t, "")
	a := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "First", "--ac", "works")))[1]
	b := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Second", "--ac", "works", "--depends", a)))[1]
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: tasks")
	escape := `aitk --json work ` + b + ` > work.json 2>&1 || true
aitk close --status blocked --summary "probe" --knowledge none
`
	aitkRun(t, dir, "--cmd", escape, "--max-attempts", "1", "--max-tasks", "1")
	if out, _ := os.ReadFile(filepath.Join(dir, "work.json")); !strings.Contains(string(out), "E_RUN_SCOPE") {
		t.Fatalf("aitk work on another task inside aitk run must fail with E_RUN_SCOPE:\n%s", out)
	}
}

const strictSetup = `f=$(ls docs/ai/tasks/"$AITK_RUN_TASK"-*.md)
mkdir -p secure && echo s > secure/x.txt
sed -i.bak 's/(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)/Impact: x. Security (STRIDE): y. Rollback: z. Validation: w./' "$f" && rm -f "$f.bak"
touch ok.txt
aitk task update --ac-done 1
aitk check
`

// A strict agent hand-writes two passes by "other" reviewers and status done, without running
// aitk close or aitk review pass. aitk run records the agent as a worker itself, so the passes
// (re-attributed to the agent) can never count as the independent review.
func TestRunStrictHandForgedPassesNeverCount(t *testing.T) {
	dir := newRunProject(t, "\n[security]\naudit = \"off\"\nasvs_level = 0\n")
	id := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Auth", "--ac", "works", "--profile", "strict")))[1]
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	agent := "set -e\n" + strictSetup + `awk '{print} /^status:/{print "review:\n  passes:\n    - n: 1\n      by: alice\n      at: \"2026-01-01T00:00:00Z\"\n      findings: 1\n    - n: 2\n      by: bob\n      at: \"2026-01-01T00:00:01Z\"\n      findings: 0"}' "$f" > "$f.tmp" && mv "$f.tmp" "$f"
sed -i.bak -E 's/^status: .*/status: done/' "$f" && rm -f "$f.bak"
`
	aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	if st := statusOf(t, dir, id); st == "done" {
		t.Fatalf("hand-written passes certified strict work: %+v", passesOf(t, dir, id))
	}
	for _, ps := range passesOf(t, dir, id) {
		if ps.By == "alice" || ps.By == "bob" {
			t.Fatalf("a hand-written reviewer name survived: %+v", passesOf(t, dir, id))
		}
	}
}

// Strict work without the agent's own clean pass is retried, never left waiting for another tool.
func TestRunStrictRetriesWithoutTheAgentsOwnCleanPass(t *testing.T) {
	for _, review := range []string{"", "aitk review pass --findings 1\n"} {
		dir := newRunProject(t, "\n[security]\naudit = \"off\"\nasvs_level = 0\n")
		must(t, dir, bin, "task", "new", "Auth", "--ac", "works", "--profile", "strict")
		must(t, dir, "git", "add", "-A")
		must(t, dir, "git", "commit", "-qm", "chore: task")
		agent := "set -e\n" + strictSetup + review + "aitk close --summary auth --knowledge none\n"
		probe := `if [ -f .attempted ]; then grep -c "The previous attempt did not finish: .*own review pass" "$AITK_RUN_PROMPT_FILE" > reason.txt || true; fi
touch .attempted
`
		r := aitkRun(t, dir, "--cmd", probe+agent, "--max-attempts", "2", "--max-tasks", "1")
		if len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Attempts != 2 || r.Data.Tasks[0].Status == "in_review" || r.Data.Tasks[0].Status == "done" {
			t.Fatalf("review %q: strict work without the agent's own clean pass must be retried, not left waiting: %+v", review, r.Data.Tasks)
		}
		if b, _ := os.ReadFile(filepath.Join(dir, "reason.txt")); strings.TrimSpace(string(b)) != "1" {
			t.Fatalf("review %q: the second attempt's prompt must say why the first failed (got %q)", review, b)
		}
		if review != "" {
			if ps := passesOf(t, dir, r.Data.Tasks[0].ID); len(ps) != 0 {
				t.Fatalf("failed strict attempts must keep none of the agent's passes: %+v", ps)
			}
		}
	}
}

// Projects without [quality] review keep the old strict behaviour in aitk run: a change that
// becomes strict waits for another tool after one attempt; no own-pass retry is imposed.
func TestRunStrictWithoutReviewLoopKeepsOldBehaviour(t *testing.T) {
	dir := newRunProject(t, "\n[security]\naudit = \"off\"\nasvs_level = 0\n")
	cfg, _ := os.ReadFile(filepath.Join(dir, "aitk.toml"))
	os.WriteFile(filepath.Join(dir, "aitk.toml"), []byte(strings.Replace(string(cfg), `review = "standard"`, "", 1)), 0o644)
	must(t, dir, bin, "task", "new", "Auth", "--ac", "works") // unpinned: the diff makes it strict
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	r := aitkRun(t, dir, "--cmd", "set -e\n"+strictSetup+"aitk close --summary auth --knowledge none\n", "--max-attempts", "1", "--max-tasks", "1")
	if len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Status != "in_review" || r.Data.Tasks[0].Attempts != 1 {
		t.Fatalf("without the review loop, strict work waits for another tool after one attempt: %+v", r.Data.Tasks)
	}
}

// A failed attempt keeps none of the agent's passes, also when the standard loop is retried.
func TestRunFailedStandardAttemptKeepsNoPasses(t *testing.T) {
	dir := newRunProject(t, "")
	id := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Feature", "--ac", "works")))[1]
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	stale := `set -e
for i in 1 2 3 4 5 6; do echo x > "src_$i.txt"; done
touch ok.txt
aitk task update --ac-done 1
aitk check
aitk review pass --findings 0
echo changed > src_1.txt
aitk check
aitk close --summary "did it" --knowledge none
`
	aitkRun(t, dir, "--cmd", stale, "--max-attempts", "1", "--max-tasks", "1")
	if ps := passesOf(t, dir, id); len(ps) != 0 {
		t.Fatalf("a failed attempt (stale clean pass) must keep no passes: %+v", ps)
	}
}

// created_by is protected: the task's creator (codex) reviewed it, which never counts as
// independent; the run agent cannot rewrite created_by to make that pass count.
func TestRunCreatedByIsProtected(t *testing.T) {
	dir := newRunProject(t, "\n[security]\naudit = \"off\"\nasvs_level = 0\n")
	id := strings.Fields(strings.TrimSpace(mustEnv(t, dir, []string{"AITK_TOOL=codex"}, bin, "task", "new", "Auth", "--ac", "works", "--profile", "strict")))[1]
	mustEnv(t, dir, []string{"AITK_TOOL=codex"}, bin, "review", "pass", id, "--findings", "0")
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	agent := "set -e\n" + strictSetup + `aitk review pass --findings 0
aitk close --summary auth --knowledge none || true
sed -i.bak -E 's/^created_by: .*/created_by: human/; s/^status: .*/status: done/' "$f" && rm -f "$f.bak"
`
	aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	if st := statusOf(t, dir, id); st == "done" {
		t.Fatalf("the creator's own pass counted as independent after created_by was rewritten: %+v", passesOf(t, dir, id))
	}
}

func mustEnv(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Env = append(os.Environ(), env...)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}

// Another tool found problems in strict work; the task was reopened and the run agent fixed it
// and reviewed itself. That is not the other tool confirming the fix: the task waits for it.
func TestRunStrictAfterIndependentFindingsWaitsForTheReviewer(t *testing.T) {
	for _, loop := range []bool{false, true} {
		dir := newRunProject(t, "\n[security]\naudit = \"off\"\nasvs_level = 0\n")
		if !loop {
			cfg, _ := os.ReadFile(filepath.Join(dir, "aitk.toml"))
			os.WriteFile(filepath.Join(dir, "aitk.toml"), []byte(strings.Replace(string(cfg), `review = "standard"`, "", 1)), 0o644)
		}
		id := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Auth", "--ac", "works", "--profile", "strict")))[1]
		mustEnv(t, dir, []string{"AITK_TOOL=reviewer"}, bin, "review", "pass", id, "--findings", "2")
		must(t, dir, bin, "task", "update", id, "--status", "todo")
		must(t, dir, "git", "add", "-A")
		must(t, dir, "git", "commit", "-qm", "chore: task")
		r := aitkRun(t, dir, "--cmd", "set -e\n"+strictSetup+"aitk review pass --findings 0\naitk close --summary auth --knowledge none\n", "--max-attempts", "1", "--max-tasks", "1")
		if len(r.Data.Tasks) != 1 || !strings.Contains(r.Data.Tasks[0].Note, "independent review pass by another tool") {
			t.Fatalf("loop=%v: the note must name the wait for another tool, not a person: %+v", loop, r.Data.Tasks)
		}
		if st := statusOf(t, dir, id); st == "done" {
			t.Fatalf("loop=%v: strict work done on the agent's own pass after the reviewer's findings: %+v", loop, passesOf(t, dir, id))
		}
	}
}

// A blocked attempt still records the agent as a worker (it wrote code for the task).
func TestRunBlockedAttemptRecordsTheAgentAsWorker(t *testing.T) {
	dir := newRunProject(t, "")
	id := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Feature", "--ac", "works")))[1]
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	agent := `echo x > half_done.txt
aitk task block "$AITK_RUN_TASK" --reason stuck
`
	aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	var r struct {
		Data struct {
			Task struct {
				Status  string   `json:"status"`
				Workers []string `json:"workers"`
			} `json:"task"`
		} `json:"data"`
	}
	json.Unmarshal([]byte(must(t, dir, bin, "--json", "task", "show", id)), &r)
	if r.Data.Task.Status != "blocked" || !strings.Contains(strings.Join(r.Data.Task.Workers, ","), "custom") {
		t.Fatalf("a blocked attempt must record the agent as a worker: %+v", r.Data.Task)
	}
}

// Changes an agent makes outside its own task during an attempt (aitk.toml, other existing
// tasks) are restored and reported; new task files it files are kept.
func TestRunRestoresChangesOutsideTheAgentsTask(t *testing.T) {
	dir := newRunProject(t, "")
	a := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "First", "--ac", "works")))[1]
	b := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Second", "--ac", "works", "--depends", a)))[1]
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: tasks")
	cfgBefore, _ := os.ReadFile(filepath.Join(dir, "aitk.toml"))
	bFile := strings.TrimSpace(must(t, dir, "sh", "-c", "ls docs/ai/tasks/"+b+"-*.md"))
	bBefore, _ := os.ReadFile(filepath.Join(dir, bFile))
	agent := `set -e
sed -i.bak 's/^review = "standard"/review = "off"/' aitk.toml && rm -f aitk.toml.bak
aitk review pass ` + b + ` --findings 0 || true
sed -i.bak 's/^title: .*/title: Hijacked/' ` + bFile + ` && rm -f ` + bFile + `.bak
aitk task new "Follow-up the agent found" > /dev/null
touch ok.txt
aitk task update --ac-done 1
aitk check
aitk close --summary "did it" --knowledge none
`
	r := aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	if cfg, _ := os.ReadFile(filepath.Join(dir, "aitk.toml")); string(cfg) != string(cfgBefore) {
		t.Fatalf("aitk.toml must be restored:\n%s", cfg)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, bFile)); string(got) != string(bBefore) {
		t.Fatalf("another task's file must be restored:\n%s", got)
	}
	if len(r.Data.Tasks) != 1 || !strings.Contains(r.Data.Tasks[0].Note, "aitk.toml") || !strings.Contains(r.Data.Tasks[0].Note, bFile) {
		t.Fatalf("the run must report what it restored: %+v", r.Data.Tasks)
	}
	if out := must(t, dir, bin, "task", "list", "--all"); !strings.Contains(out, "Follow-up the agent found") {
		t.Fatalf("a new task the agent filed must be kept:\n%s", out)
	}
}

// Tasks are found by id, not by file name: a new file that claims another task's id must not
// take that task over, and aitk itself refuses to write other tasks inside a run.
func TestRunRemovesFilesClaimingAnotherTasksID(t *testing.T) {
	dir := newRunProject(t, "")
	a := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "First", "--ac", "works")))[1]
	b := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Second", "--ac", "works", "--depends", a)))[1]
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: tasks")
	agent := `f=$(ls docs/ai/tasks/` + b + `-*.md)
sed -e 's/^title: .*/title: Hijacked/' -e 's/^status: .*/status: cancelled/' -e 's/^updated: .*/updated: "2099-01-01T00:00:00Z"/' "$f" > docs/ai/tasks/zz-copy.md
aitk --json task update ` + b + ` --add-ac "sneaky" > other-write.json 2>&1
touch ok.txt
aitk task update --ac-done 1
aitk check
aitk close --summary "did it" --knowledge none
`
	r := aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	if out, _ := os.ReadFile(filepath.Join(dir, "other-write.json")); !strings.Contains(string(out), "E_RUN_SCOPE") {
		t.Fatalf("aitk must refuse writing another task inside a run:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs/ai/tasks/zz-copy.md")); err == nil {
		t.Fatal("a file claiming another task's id must be removed")
	}
	if out := must(t, dir, bin, "task", "show", b); strings.Contains(out, "Hijacked") || strings.Contains(out, "sneaky") {
		t.Fatalf("the other task must be untouched:\n%s", out)
	}
	if len(r.Data.Tasks) != 1 || !strings.Contains(r.Data.Tasks[0].Note, "duplicate of "+b) {
		t.Fatalf("the run must report the removed duplicate: %+v", r.Data.Tasks)
	}
}

// A task the agent files during an attempt keeps only its description: seeded reviews, evidence,
// workers and a finished status are stripped, so it cannot arrive pre-certified.
func TestRunNewTasksArriveAsPlainWork(t *testing.T) {
	dir := newRunProject(t, "")
	must(t, dir, bin, "task", "new", "First", "--ac", "works")
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	agent := `n=$(aitk task new "Auth follow-up" --profile strict | awk 'NR==1{print $2}')
echo "$n" > new-id.txt
f=$(ls docs/ai/tasks/"$n"-*.md)
awk '{print} /^status:/{print "review:\n  passes:\n    - n: 1\n      by: codex\n      at: \"2026-01-01T00:00:00Z\"\n      findings: 0\n    - n: 2\n      by: gemini-cli\n      at: \"2026-01-01T00:00:01Z\"\n      findings: 0\nworkers:\n  - nobody"}' "$f" > "$f.tmp" && mv "$f.tmp" "$f"
sed -i.bak -E 's/^status: .*/status: in_review/; s/^created_by: .*/created_by: human/' "$f" && rm -f "$f.bak"
touch ok.txt
aitk task update --ac-done 1
aitk check
aitk close --summary "did it" --knowledge none
`
	aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	id, _ := os.ReadFile(filepath.Join(dir, "new-id.txt"))
	n := strings.TrimSpace(string(id))
	var r struct {
		Data struct {
			Task struct {
				Status    string   `json:"status"`
				CreatedBy string   `json:"created_by"`
				Workers   []string `json:"workers"`
			} `json:"task"`
		} `json:"data"`
	}
	json.Unmarshal([]byte(must(t, dir, bin, "--json", "task", "show", n)), &r)
	if ps := passesOf(t, dir, n); len(ps) != 0 || r.Data.Task.Status != "todo" || r.Data.Task.CreatedBy != "custom" || len(r.Data.Task.Workers) != 0 {
		t.Fatalf("a new task must arrive as plain work: %+v passes %+v", r.Data.Task, ps)
	}
}

// The repository's git hooks are files the agent can write: aitk run commits without them.
func TestRunCommitsWithoutTheRepositorysHooks(t *testing.T) {
	dir := newRunProject(t, "")
	must(t, dir, bin, "task", "new", "First", "--ac", "works")
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	agent := `set -e
printf '#!/bin/sh\nsed -i.bak "s/^review = \\"standard\\"/review = \\"off\\"/" aitk.toml && rm -f aitk.toml.bak\ngit add aitk.toml\n' > .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
echo x > feature.txt
touch ok.txt
aitk task update --ac-done 1
aitk check
aitk close --summary "did it" --knowledge none
`
	aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if cfg, _ := os.ReadFile(filepath.Join(dir, "aitk.toml")); !strings.Contains(string(cfg), `review = "standard"`) {
		t.Fatalf("a hook must not change aitk.toml during the run's commit:\n%s", cfg)
	}
	if out := must(t, dir, "git", "log", "-1", "--stat"); strings.Contains(out, "aitk.toml") {
		t.Fatalf("the run's commit must not carry a hook's change:\n%s", out)
	}
}

func twoTasks(t *testing.T, dir string) (string, string, string, []byte) {
	t.Helper()
	a := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "First", "--ac", "works")))[1]
	b := strings.Fields(strings.TrimSpace(must(t, dir, bin, "task", "new", "Second", "--ac", "works", "--depends", a)))[1]
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: tasks")
	bFile := strings.TrimSpace(must(t, dir, "sh", "-c", "ls docs/ai/tasks/"+b+"-*.md"))
	bWas, _ := os.ReadFile(filepath.Join(dir, bFile))
	return a, b, bFile, bWas
}

func sameFile(t *testing.T, dir, rel string, want []byte, what string) {
	t.Helper()
	if got, err := os.ReadFile(filepath.Join(dir, rel)); err != nil || string(got) != string(want) {
		t.Fatalf("%s: %s must be restored (err %v):\n%s", what, rel, err, got)
	}
}

// Security review pass 2 scenarios: each must leave the other task exactly as it was.
func TestRunGuardSecondPass(t *testing.T) {
	close := "touch ok.txt\naitk task update --ac-done 1\naitk check\naitk close --summary done --knowledge none\n"

	// The agent rewrites its own task file to claim the other task's id.
	dir := newRunProject(t, "")
	a, b, bFile, bWas := twoTasks(t, dir)
	aFile := strings.TrimSpace(must(t, dir, "sh", "-c", "ls docs/ai/tasks/"+a+"-*.md"))
	aitkRun(t, dir, "--cmd", `sed -i.bak -e 's/^id: .*/id: `+b+`/' -e 's/^status: .*/status: done/' -e 's/^updated: .*/updated: "2099-01-01T00:00:00Z"/' `+aFile+` && rm -f `+aFile+`.bak`, "--max-attempts", "1", "--max-tasks", "1")
	if out := must(t, dir, bin, "task", "show", b); strings.Contains(out, "status: done") {
		t.Fatalf("own file claiming another id took the task over:\n%s", out)
	}
	sameFile(t, dir, bFile, bWas, "own-file id")

	// A new task that fails validation (or uses a look-alike id) is removed, not kept as written.
	dir = newRunProject(t, "")
	_, b, bFile, bWas = twoTasks(t, dir)
	// Written after the agent's last aitk call, so aitk's self-heal never sees them: the guard must.
	agent := close + `f=$(ls docs/ai/tasks/` + b + `-*.md)
sed -e 's/^id: .*/id: T-ſss1/' -e 's/^status: .*/status: done/' "$f" > docs/ai/tasks/zz-lookalike.md
printf -- '---\nid: T-zz99\ntitle: ab\nstatus: done\nprofile: strict\ncreated: "2026-01-01T00:00:00Z"\nupdated: "2026-01-01T00:00:00Z"\nreview:\n  passes:\n    - n: 1\n      by: codex\n      at: "2026-01-01T00:00:00Z"\n      findings: 0\n---\nx\n' > docs/ai/tasks/zz-invalid.md
`
	aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	for _, f := range []string{"zz-lookalike.md", "zz-invalid.md"} {
		if _, err := os.Stat(filepath.Join(dir, "docs/ai/tasks", f)); err == nil {
			if out := must(t, dir, "sh", "-c", "cat docs/ai/tasks/"+f); strings.Contains(out, "codex") || strings.Contains(out, "status: done") {
				t.Fatalf("%s survived with its seeded state:\n%s", f, out)
			}
		}
	}
	sameFile(t, dir, bFile, bWas, "look-alike id")

	// git config written by the agent (fsmonitor) must not run when aitk commits.
	dir = newRunProject(t, "")
	_, b, bFile, bWas = twoTasks(t, dir)
	agent = `printf '#!/bin/sh\nsed -i.bak "s/^status: .*/status: done/" ` + bFile + ` && rm -f ` + bFile + `.bak\n' > .git/fsm.sh
chmod +x .git/fsm.sh
git config core.fsmonitor "$PWD/.git/fsm.sh"
echo x > feature.txt
` + close
	aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	sameFile(t, dir, bFile, bWas, "fsmonitor")
	if out, _ := sh(t, dir, "git", "config", "--get-all", "core.fsmonitor"); strings.TrimSpace(out) != "" {
		t.Fatalf("the agent's git config must be restored, core.fsmonitor = %q", out)
	}

	// A broken project lock must not switch the restore off; a directory in place of a task file
	// is replaced by the file.
	dir = newRunProject(t, "")
	_, b, bFile, bWas = twoTasks(t, dir)
	agent = `sed -i.bak 's/^status: .*/status: done/' ` + bFile + ` && rm -f ` + bFile + `.bak
lock=$(git rev-parse --git-common-dir)/aitk/lock
rm -rf "$lock"; mkdir -p "$lock"
`
	aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	sameFile(t, dir, bFile, bWas, "broken lock")
	dir = newRunProject(t, "")
	_, b, bFile, bWas = twoTasks(t, dir)
	aitkRun(t, dir, "--cmd", "rm -f "+bFile+" && mkdir "+bFile+"\n"+close, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	sameFile(t, dir, bFile, bWas, "directory in place of a task file")

	// Project code run by the check edits another task: restored after verification. And aitk
	// review pass refuses another task inside a run.
	dir = newRunProject(t, "")
	cfg, _ := os.ReadFile(filepath.Join(dir, "aitk.toml"))
	os.WriteFile(filepath.Join(dir, "aitk.toml"), []byte(strings.Replace(string(cfg), `cmd = "`+check()+`"`, `cmd = "sh check.sh"`, 1)), 0o644)
	if !strings.Contains(must(t, dir, "cat", "aitk.toml"), "sh check.sh") {
		t.Skip("could not point the check at check.sh in this environment")
	}
	_, b, bFile, bWas = twoTasks(t, dir)
	agent = `printf 'sed -i.bak "s/^status: .*/status: done/" ` + bFile + ` 2>/dev/null; rm -f ` + bFile + `.bak; exit 0\n' > check.sh
aitk --json review pass ` + b + ` --findings 0 > review-other.json 2>&1
aitk task update --ac-done 1
aitk check
aitk close --summary done --knowledge none
`
	aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	sameFile(t, dir, bFile, bWas, "check edits another task")
	if out, _ := os.ReadFile(filepath.Join(dir, "review-other.json")); !strings.Contains(string(out), "E_RUN_SCOPE") {
		t.Fatalf("aitk review pass on another task inside a run must fail with E_RUN_SCOPE:\n%s", out)
	}
}

// Glob characters in the repository path must not switch the duplicate check off.
func TestRunGuardWorksWithGlobCharactersInThePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake agents are shell scripts")
	}
	dir := filepath.Join(t.TempDir(), "proj[1]")
	os.MkdirAll(dir, 0o755)
	must(t, dir, "git", "init", "-q", "-b", "main")
	must(t, dir, "git", "config", "user.email", "t@example.com")
	must(t, dir, "git", "config", "user.name", "t")
	must(t, dir, bin, "init", "--check", check())
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: init")
	_, b, bFile, bWas := twoTasks(t, dir)
	agent := `sed -e 's/^status: .*/status: cancelled/' -e 's/^updated: .*/updated: "2099-01-01T00:00:00Z"/' ` + bFile + ` > docs/ai/tasks/zz-copy.md
aitk close --status blocked --summary probe --knowledge none
`
	aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	if _, err := os.Stat(filepath.Join(dir, "docs/ai/tasks/zz-copy.md")); err == nil {
		t.Fatalf("a copy claiming %s survived in a path with glob characters", b)
	}
	sameFile(t, dir, bFile, bWas, "glob characters in the path")
}

// Security review pass 3 scenarios.
func TestRunGuardThirdPass(t *testing.T) {
	close := "touch ok.txt\naitk task update --ac-done 1\naitk check\naitk close --summary done --knowledge none\n"
	committed := func(t *testing.T, dir, rel string) string {
		t.Helper()
		out, _ := sh(t, dir, "git", "show", "HEAD:"+rel)
		return out
	}

	// Staged content hidden with skip-worktree must not reach the commit.
	dir := newRunProject(t, "")
	_, _, bFile, bWas := twoTasks(t, dir)
	agent := `sed -i.bak 's/^status: .*/status: cancelled/' ` + bFile + ` && rm -f ` + bFile + `.bak
git add ` + bFile + `
git update-index --skip-worktree ` + bFile + `
echo x > feature.txt
` + close
	aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if got := committed(t, dir, bFile); got != string(bWas) {
		t.Fatalf("skip-worktree content reached the commit:\n%s", got)
	}

	// A guarded file swapped for a symlink with the same content is put back as a file.
	dir = newRunProject(t, "")
	twoTasks(t, dir)
	outside := t.TempDir()
	agent = `cp aitk.toml ` + outside + `/cfg.toml && rm aitk.toml && ln -s ` + outside + `/cfg.toml aitk.toml
echo x > feature.txt
` + close
	aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if fi, err := os.Lstat(filepath.Join(dir, "aitk.toml")); err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("aitk.toml must be a regular file again: %v %v", fi, err)
	}
	if out := must(t, dir, "git", "ls-tree", "HEAD", "aitk.toml"); strings.HasPrefix(out, "120000") {
		t.Fatalf("a symlink was committed in place of aitk.toml: %s", out)
	}

	// Pointing git at another common dir (with its own config and filters) stops the commit.
	dir = newRunProject(t, "")
	_, _, bFile, bWas = twoTasks(t, dir)
	evil := t.TempDir()
	agent = `cp -r .git/. ` + evil + `/
printf '[filter "x"]\n\tclean = sh -c "sed -i.bak s/^status:.*/status:cancelled/ ` + bFile + `; cat"\n' >> ` + evil + `/config
echo ` + evil + ` > .git/commondir
echo '*.txt filter=x' > .gitattributes
echo x > feature.txt
` + close
	aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	sameFile(t, dir, bFile, bWas, "commondir redirect")
	if _, err := os.Stat(filepath.Join(dir, ".git", "commondir")); err == nil {
		t.Fatal("a commondir file the agent created must be removed")
	}

	// Two new files with the same new id: the second is a duplicate. A case-variant of an
	// existing id is a duplicate too (ids compare like task lookup).
	dir = newRunProject(t, "")
	_, b, bFile, bWas := twoTasks(t, dir)
	agent = `n=$(aitk task new "Follow up" | awk 'NR==1{print $2}')
f=$(ls docs/ai/tasks/"$n"-*.md)
` + close + `sed 's/^title: .*/title: Twin of the follow up/' "$f" > docs/ai/tasks/zz-twin.md
sed -e 's/^id: .*/id: ` + strings.ToLower(b) + `/' -e 's/^status: .*/status: cancelled/' -e 's/^updated: .*/updated: "2099-01-01T00:00:00Z"/' ` + bFile + ` > docs/ai/tasks/zz-case.md
`
	aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	if _, err := os.Stat(filepath.Join(dir, "docs/ai/tasks/zz-twin.md")); err == nil {
		t.Fatal("a second new file with the same new id must be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "docs/ai/tasks/zz-case.md")); err == nil {
		t.Fatal("a case-variant of an existing id must be removed")
	}
	sameFile(t, dir, bFile, bWas, "case-variant id")
	if out := must(t, dir, bin, "task", "list", "--all"); !strings.Contains(out, "Follow up") {
		t.Fatalf("the first new task must be kept:\n%s", out)
	}

	// What the guard cannot restore stops the commit: the tasks folder replaced by a symlink.
	dir = newRunProject(t, "")
	twoTasks(t, dir)
	moved := t.TempDir()
	agent = close + `cp -r docs/ai/tasks/. ` + moved + `/ && rm -rf docs/ai/tasks && ln -s ` + moved + ` docs/ai/tasks
echo x > feature.txt
`
	before := strings.TrimSpace(must(t, dir, "git", "rev-parse", "HEAD"))
	r := aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if after := strings.TrimSpace(must(t, dir, "git", "rev-parse", "HEAD")); after != before {
		t.Fatalf("a commit was made although the guard could not restore the tasks folder: %+v", r)
	}
}

// Security review pass 4 scenarios.
func TestRunGuardFourthPass(t *testing.T) {
	close := "touch ok.txt\naitk task update --ac-done 1\naitk check\naitk close --summary done --knowledge none\n"
	head := func(dir string) string { return strings.TrimSpace(must(t, dir, "git", "rev-parse", "HEAD")) }

	// A change the guard cannot restore ends the task and the run at once, also without
	// --commit: a second attempt must not start from it, nor the next task.
	for _, mode := range [][]string{nil, {"--commit"}} {
		dir := newRunProject(t, "")
		_, _, bFile, _ := twoTasks(t, dir)
		out := t.TempDir()
		agent := `sed -i.bak 's/^status: .*/status: cancelled/' ` + bFile + ` && rm -f ` + bFile + `.bak
mv docs/ai ` + out + `/ai && ln -s ` + out + `/ai docs/ai
`
		before := head(dir)
		r := aitkRun(t, dir, append([]string{"--cmd", agent, "--max-attempts", "2", "--max-tasks", "3"}, mode...)...)
		if len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Attempts != 1 || !strings.Contains(r.Data.Stopped, "could not restore") ||
			!strings.Contains(r.Data.Tasks[0].Note, "docs/ai") {
			t.Fatalf("mode %v: an unrestorable change must stop the task and the run: %+v", mode, r)
		}
		if head(dir) != before {
			t.Fatalf("mode %v: nothing may be committed", mode)
		}
	}

	// A symlinked parent of the tasks folder (docs/ai) stops the commit.
	dir := newRunProject(t, "")
	twoTasks(t, dir)
	out := t.TempDir()
	agent := close + `cp -r docs/ai/. ` + out + `/ && rm -rf docs/ai && ln -s ` + out + ` docs/ai
echo x > feature.txt
`
	before := head(dir)
	r := aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if head(dir) != before || !strings.Contains(r.Data.Stopped, "could not restore") {
		t.Fatalf("a symlinked docs/ai must stop the commit: %+v", r)
	}

	// A .git directory replaced by a gitdir: file stops the commit.
	dir = newRunProject(t, "")
	twoTasks(t, dir)
	gd := t.TempDir()
	agent = close + `mv .git ` + gd + `/g && echo "gitdir: ` + gd + `/g" > .git
echo x > feature.txt
`
	before = head(dir)
	r = aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if head(dir) != before || !strings.Contains(r.Data.Stopped, "could not restore") {
		t.Fatalf("a replaced .git directory must stop the commit: %+v", r)
	}

	// A config file in the working tree that git includes is guarded: a filter the agent
	// defines there cannot rewrite another task in aitk's own commit.
	dir = newRunProject(t, "")
	write(t, dir, "repo.gitconfig", "[core]\n\tautocrlf = false\n")
	must(t, dir, "git", "config", "include.path", "../repo.gitconfig")
	_, _, bFile, bWas := twoTasks(t, dir)
	agent = `printf '[filter "x"]\n\tclean = sed s/^status:.*/status:\\\\ cancelled/\n' >> repo.gitconfig
echo 'docs/ai/tasks/*.md filter=x' > .gitattributes
echo x > feature.txt
` + close
	aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if got, _ := sh(t, dir, "git", "show", "HEAD:"+bFile); got != string(bWas) {
		t.Fatalf("an included config's filter rewrote another task in the commit:\n%s", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "repo.gitconfig")); strings.Contains(string(b), "filter") {
		t.Fatalf("the included config must be restored:\n%s", b)
	}
}

// Security review pass 5 scenarios: a filter defined in any config the repository reads (an
// include that is missing, empty, branch-conditional, inside .git, or in the main checkout of a
// linked worktree) never rewrites another task in aitk's commit; a symlink inside the
// repository on the tasks path keeps working.
func TestRunGuardFifthPass(t *testing.T) {
	close := "touch ok.txt\naitk task update --ac-done 1\naitk check\naitk close --summary done --knowledge none\n"
	filter := func(cfg string) string {
		return `printf '[filter "x"]\n\tclean = sed s/^status:.*/status:\\\\ cancelled/\n' >> ` + cfg + `
echo 'docs/ai/tasks/*.md filter=x' > .gitattributes
echo x > feature.txt
`
	}
	untouched := func(t *testing.T, dir, bFile string, bWas []byte, what string) {
		t.Helper()
		if got, _ := sh(t, dir, "git", "show", "HEAD:"+bFile); got != string(bWas) {
			t.Fatalf("%s: a filter rewrote another task in the commit:\n%s", what, got)
		}
	}
	cases := []struct {
		name, setup, agent string
	}{
		{"missing include", "git config include.path ../repo.gitconfig", filter("repo.gitconfig")},
		{"empty include", "echo '# shared settings' > repo.gitconfig && git add repo.gitconfig && git commit -qm 'chore: cfg' && git config include.path ../repo.gitconfig", filter("repo.gitconfig")},
		{"branch include", "git config includeIf.onbranch:evil.path ../evil.gitconfig", "git checkout -q -b evil\n" + filter("evil.gitconfig")},
		{"include in .git", "printf '[core]\\n\\tautocrlf = false\\n' > .git/extra.cfg && git config include.path extra.cfg", filter(".git/extra.cfg")},
	}
	for _, c := range cases {
		dir := newRunProject(t, "")
		must(t, dir, "sh", "-c", c.setup)
		_, _, bFile, bWas := twoTasks(t, dir)
		aitkRun(t, dir, "--cmd", c.agent+close, "--commit", "--max-attempts", "1", "--max-tasks", "1")
		untouched(t, dir, bFile, bWas, c.name)
	}

	// The main checkout's included config, run from a linked worktree.
	dir := newRunProject(t, "")
	write(t, dir, "repo.gitconfig", "[core]\n\tautocrlf = false\n")
	must(t, dir, "git", "add", "repo.gitconfig")
	must(t, dir, "git", "commit", "-qm", "chore: cfg")
	must(t, dir, "git", "config", "include.path", "../repo.gitconfig")
	wt := filepath.Join(t.TempDir(), "wt")
	must(t, dir, "git", "worktree", "add", "-q", "-b", "wt", wt)
	_, _, bFile, bWas := twoTasks(t, wt)
	aitkRun(t, wt, "--cmd", filter(filepath.Join(dir, "repo.gitconfig"))+close, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	untouched(t, wt, bFile, bWas, "linked worktree include")

	// docs -> documentation inside the repository is a legitimate layout: the guard does not
	// treat it as a link out of the repository.
	dir = newRunProject(t, "")
	must(t, dir, "sh", "-c", "mv docs documentation && ln -s documentation docs && git add -A && git commit -qm 'chore: move docs'")
	twoTasks(t, dir)
	r := aitkRun(t, dir, "--cmd", "echo x > feature.txt", "--max-attempts", "1", "--max-tasks", "1")
	if len(r.Data.Tasks) == 0 || strings.Contains(r.Data.Stopped, "could not restore") || strings.Contains(r.Data.Tasks[0].Note, "link") {
		t.Fatalf("a symlink inside the repository must not stop the run: %+v", r)
	}
}

// Security review pass 6 scenarios.
func TestRunGuardSixthPass(t *testing.T) {
	close := "touch ok.txt\naitk task update --ac-done 1\naitk check\naitk close --summary done --knowledge none\n"
	head := func(dir string) string { return strings.TrimSpace(must(t, dir, "git", "rev-parse", "HEAD")) }

	// Sequencer state the agent leaves behind (an author from CHERRY_PICK_HEAD, an extra parent
	// from MERGE_HEAD) never shapes aitk's commit: the commit stops instead.
	for _, f := range []string{"CHERRY_PICK_HEAD", "MERGE_HEAD"} {
		dir := newRunProject(t, "")
		twoTasks(t, dir)
		write(t, dir, "m.txt", "m")
		must(t, dir, "git", "add", "m.txt")
		must(t, dir, "git", "-c", "user.name=Maintainer Person", "-c", "user.email=maint@example.com", "commit", "-qm", "chore: maintainer")
		before := head(dir)
		agent := "echo x > feature.txt\n" + close + "git rev-parse HEAD > .git/" + f + "\n"
		r := aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
		if head(dir) != before || !strings.Contains(r.Data.Tasks[0].Note, f) {
			t.Fatalf("%s: aitk must not commit with sequencer state present: %+v\n%s", f, r, must(t, dir, "git", "log", "-2", "--format=%an %p"))
		}
	}

	// An include that is a dotfiles symlink outside the repository is never rewritten.
	dir := newRunProject(t, "")
	home := t.TempDir()
	write(t, home, "dotfiles/git-work.gc", "[core]\n\tautocrlf = false\n")
	must(t, home, "ln", "-s", filepath.Join(home, "dotfiles/git-work.gc"), "git-work.gc")
	must(t, dir, "git", "config", "include.path", filepath.Join(home, "git-work.gc"))
	twoTasks(t, dir)
	r := aitkRun(t, dir, "--cmd", "echo x > feature.txt\n"+close, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if fi, err := os.Lstat(filepath.Join(home, "git-work.gc")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the user's symlinked include must stay a symlink: %v %v", fi, err)
	}
	if len(r.Data.Tasks) == 0 || r.Data.Tasks[0].Status != "done" || strings.Contains(r.Data.Tasks[0].Note, "git-work") {
		t.Fatalf("an untouched include must not be reported or stop the run: %+v", r)
	}

	// aitk never writes outside the repository: an agent that edits such an include stops the
	// run (the effective config changed) instead of having aitk rewrite the user's file.
	dir = newRunProject(t, "")
	home = t.TempDir()
	write(t, home, "user.gc", "[core]\n\tautocrlf = false\n")
	must(t, dir, "git", "config", "include.path", filepath.Join(home, "user.gc"))
	twoTasks(t, dir)
	before := head(dir)
	r = aitkRun(t, dir, "--cmd", "echo x > feature.txt\nprintf '[filter \"x\"]\\n\\tclean = cat\\n' >> "+filepath.Join(home, "user.gc")+"\n"+close, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if b, _ := os.ReadFile(filepath.Join(home, "user.gc")); !strings.Contains(string(b), "filter") {
		t.Fatalf("aitk must not rewrite a file outside the repository:\n%s", b)
	}
	if head(dir) != before || !strings.Contains(r.Data.Stopped, "could not restore") {
		t.Fatalf("a changed include outside the repository must stop the run: %+v", r)
	}

	// A symlinked .git keeps working and is never removed; a symlinked .git/config stays a link.
	dir = newRunProject(t, "")
	store := filepath.Join(t.TempDir(), "store.git")
	must(t, dir, "sh", "-c", "mv .git "+store+" && ln -s "+store+" .git")
	must(t, dir, "sh", "-c", "mv .git/config .git/config.real && ln -s config.real .git/config")
	twoTasks(t, dir)
	r = aitkRun(t, dir, "--cmd", "echo x > feature.txt\n"+close, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if fi, err := os.Lstat(filepath.Join(dir, ".git")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("a symlinked .git must stay: %v %v (%+v)", fi, err, r)
	}
	if fi, err := os.Lstat(filepath.Join(store, "config")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("a symlinked .git/config must stay a link: %v %v", fi, err)
	}
	if len(r.Data.Tasks) == 0 || r.Data.Tasks[0].Status != "done" || r.Data.Tasks[0].Commit == "" {
		t.Fatalf("a run in a repository with a symlinked .git must finish and commit: %+v", r)
	}

	// An agent that edits a symlinked .git/config has it restored through the link.
	dir = newRunProject(t, "")
	must(t, dir, "sh", "-c", "mv .git/config .git/config.real && ln -s config.real .git/config")
	_, _, bFile, bWas := twoTasks(t, dir)
	was, _ := os.ReadFile(filepath.Join(dir, ".git/config.real"))
	agent := `printf '[filter "x"]\n\tclean = sed s/^status:.*/status:\\\\ cancelled/\n' >> .git/config
echo 'docs/ai/tasks/*.md filter=x' > .gitattributes
echo x > feature.txt
` + close
	aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if got, _ := os.ReadFile(filepath.Join(dir, ".git/config.real")); string(got) != string(was) {
		t.Fatalf("the symlinked config must be restored through its link:\n%s", got)
	}
	if got, _ := sh(t, dir, "git", "show", "HEAD:"+bFile); got != string(bWas) {
		t.Fatalf("a filter in a symlinked config rewrote another task:\n%s", got)
	}
}

// Security review pass 7 scenarios.
func TestRunGuardSeventhPass(t *testing.T) {
	close := "touch ok.txt\naitk task update --ac-done 1\naitk check\naitk close --summary done --knowledge none\n"
	head := func(dir string) string { // read the ref directly: a broken config stops git itself
		b, _ := os.ReadFile(filepath.Join(dir, ".git/refs/heads/main"))
		return strings.TrimSpace(string(b))
	}
	linkedConfig := func(t *testing.T) (string, []byte) {
		dir := newRunProject(t, "")
		must(t, dir, "sh", "-c", "mv .git/config .git/config.real && ln -s config.real .git/config")
		twoTasks(t, dir)
		cfg, _ := os.ReadFile(filepath.Join(dir, "aitk.toml"))
		return dir, cfg
	}

	// A link chain re-pointed at aitk.toml is never written through.
	dir, cfg := linkedConfig(t)
	before := head(dir)
	r := aitkRun(t, dir, "--cmd", "echo x > feature.txt\nmv .git/config.real ../saved-config && ln -s ../aitk.toml .git/config.real\n"+close, "--commit", "--max-attempts", "2", "--max-tasks", "1")
	sameFile(t, dir, "aitk.toml", cfg, "re-pointed link chain")
	if head(dir) != before || !strings.Contains(r.Data.Stopped, "could not restore") {
		t.Fatalf("a re-pointed link chain must stop the run: %+v", r)
	}

	// A link chain re-pointed at a tracked copy with the same content stops the run too.
	dir, _ = linkedConfig(t)
	before = head(dir)
	r = aitkRun(t, dir, "--cmd", "echo x > feature.txt\ncp .git/config.real evil.cfg && rm .git/config.real && ln -s ../evil.cfg .git/config.real\n"+close, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if head(dir) != before || !strings.Contains(r.Data.Stopped, "could not restore") {
		t.Fatalf("a config link moved into the working tree must stop the run: %+v", r)
	}

	// A .gitattributes encoding rule cannot change another task as it is committed.
	dir = newRunProject(t, "")
	_, _, bFile, bWas := twoTasks(t, dir)
	before = head(dir)
	r = aitkRun(t, dir, "--cmd", "echo x > feature.txt\necho '"+bFile+" working-tree-encoding=IBM037' > .gitattributes\n"+close, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if got, _ := sh(t, dir, "git", "show", "HEAD:"+bFile); got != string(bWas) && head(dir) != before {
		t.Fatalf("an encoding rule changed another task in the commit: %+v\n%q", r, got)
	}
	if head(dir) != before && r.Data.Tasks[0].Commit != "" {
		t.Fatalf("the commit must stop when git would store a guarded file differently: %+v", r)
	}
}

// Security review pass 8 scenarios.
func TestRunGuardEighthPass(t *testing.T) {
	close := "touch ok.txt\naitk task update --ac-done 1\naitk check\naitk close --summary done --knowledge none\n"
	head := func(dir string) string {
		b, _ := os.ReadFile(filepath.Join(dir, ".git/refs/heads/main"))
		return strings.TrimSpace(string(b))
	}

	// An encoding rule on the agent's own task file cannot change what git stores for it.
	dir := newRunProject(t, "")
	_, _, _, _ = twoTasks(t, dir)
	before := head(dir)
	agent := "echo x > feature.txt\n" + close + `f=$(grep -l "^status: done" docs/ai/tasks/*.md | head -1)
tr -cd '\11\12\15\40-\176' < "$f" > "$f.tmp" && mv "$f.tmp" "$f"
printf '\nnote: a+AEA-b\n' >> "$f"
echo "$f working-tree-encoding=UTF-7" > .gitattributes
`
	r := aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if head(dir) != before || !strings.Contains(r.Data.Tasks[0].Note, "differently") {
		t.Fatalf("git must not store the agent's own task file differently from the file: %+v", r)
	}

	// A task file the agent leaves invalid is put back as it was before the task, and the run
	// stops: none of what it wrote (grants included) survives.
	dir = newRunProject(t, "")
	twoTasks(t, dir)
	before = head(dir)
	long := strings.Repeat("x", 130)
	agent = "echo x > feature.txt\n" + close + `f=$(grep -l "^status: done" docs/ai/tasks/*.md | head -1)
sed -i.bak "s/^title: .*/title: ` + long + `/" "$f" && rm -f "$f.bak"
`
	r = aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "2", "--max-tasks", "1")
	if head(dir) != before || !strings.Contains(r.Data.Stopped, "could not restore") {
		t.Fatalf("an invalid task file must stop the run without a commit: %+v", r)
	}
	if out, _ := sh(t, dir, "grep", "-rl", long, "docs/ai/tasks"); strings.TrimSpace(out) != "" {
		t.Fatalf("the invalid task file must be put back: %s", out)
	}

	// A hard link the agent puts at a guarded symlink's target is not written through.
	for i := 0; i < 6; i++ {
		dir = newRunProject(t, "")
		must(t, dir, "sh", "-c", "mkdir cfg && mv aitk.toml cfg/aitk.toml && ln -s cfg/aitk.toml aitk.toml && git add -A && git commit -qm 'chore: cfg'")
		_, _, bFile, bWas := twoTasks(t, dir)
		aitkRun(t, dir, "--cmd", "ln -f "+bFile+" cfg/aitk.toml\n", "--max-attempts", "1", "--max-tasks", "1")
		sameFile(t, dir, bFile, bWas, "hard link at a symlink target")
	}
}

// Security review pass 9 scenarios.
func TestRunGuardNinthPass(t *testing.T) {
	close := "touch ok.txt\naitk task update --ac-done 1\naitk check\naitk close --summary done --knowledge none\n"

	// An agent that "fails to start" after writing its own task file keeps none of it.
	dir := newRunProject(t, "")
	a, _, _, _ := twoTasks(t, dir)
	agent := "AITK_TOOL=alice aitk review pass --findings 0 >/dev/null 2>&1\nAITK_TOOL=alice aitk task update --status in_progress >/dev/null 2>&1\necho 'error: not logged in'\nexit 1\n"
	r := aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	if !strings.Contains(r.Data.Stopped, "failed to start") {
		t.Fatalf("setup: the run must take the failed-start path: %+v", r)
	}
	for _, ps := range passesOf(t, dir, a) {
		if ps.By == "alice" {
			t.Fatalf("a pass written before a failed start must not survive: %+v", passesOf(t, dir, a))
		}
	}

	// base is protected: committing the work and moving base to HEAD does not lower strict to lite.
	dir = newRunProject(t, "\n[security]\naudit = \"off\"\nasvs_level = 0\n")
	must(t, dir, bin, "task", "new", "Feature", "--ac", "works")
	must(t, dir, "git", "add", "-A")
	must(t, dir, "git", "commit", "-qm", "chore: task")
	agent = `mkdir -p secure && echo x > secure/x.txt && touch ok.txt
aitk task update --ac-done 1
git add -A && git commit -qm "feat: secure" --no-verify
f=$(ls docs/ai/tasks/*.md | head -1)
sed -i.bak -e "s/^base: .*/base: $(git rev-parse HEAD)/" -e "s/^status: .*/status: done/" "$f" && rm -f "$f.bak"
`
	r = aitkRun(t, dir, "--cmd", agent, "--max-attempts", "1", "--max-tasks", "1")
	if len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Status == "done" {
		t.Fatalf("moving base must not lower strict work to lite: %+v", r)
	}

	// A file of aitk's state that the agent committed itself is still checked against the disk.
	dir = newRunProject(t, "")
	twoTasks(t, dir)
	before := strings.TrimSpace(must(t, dir, "git", "rev-parse", "HEAD"))
	agent = "echo x > feature.txt\n" + close + `printf 'a@b\n' > docs/ai/notes.md
git add docs/ai/notes.md && git commit -qm "docs: notes" --no-verify
printf 'a+AEA-b\n' > docs/ai/notes.md
echo 'docs/ai/notes.md working-tree-encoding=UTF-7' > .gitattributes
`
	r = aitkRun(t, dir, "--cmd", agent, "--commit", "--max-attempts", "1", "--max-tasks", "1")
	if len(r.Data.Tasks) != 1 || r.Data.Tasks[0].Commit != "" || !strings.Contains(r.Data.Tasks[0].Note, "differently") {
		t.Fatalf("aitk must not commit when a committed state file differs from the disk: %+v (HEAD was %s)", r, before)
	}
}
