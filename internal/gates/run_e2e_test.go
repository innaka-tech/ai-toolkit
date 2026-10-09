package gates_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type runOut struct {
	OK   bool `json:"ok"`
	Data struct {
		Planned []struct {
			ID string `json:"id"`
		} `json:"planned"`
		Tasks []struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			Attempts int    `json:"attempts"`
			Handoff  string `json:"handoff"`
			Commit   string `json:"commit"`
			Note     string `json:"note"`
		} `json:"tasks"`
		Stopped string `json:"stopped"`
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
