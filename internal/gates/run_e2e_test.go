package gates_test

import (
	"encoding/json"
	"os"
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
		if x.Status != "done" || x.Attempts != 1 {
			t.Fatalf("task not done on the first attempt: %+v", x)
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
