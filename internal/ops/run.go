package ops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/innaka-tech/ai-toolkit/v2/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/v2/internal/brief"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/profile"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/session"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
	"github.com/innaka-tech/ai-toolkit/v2/internal/textx"
)

// headless builds argv that runs a tool non-interactively on one prompt. The defaults let the
// agent edit files and run aitk and read-only git; anything more is the user's choice ([run].args).
var headless = map[string]func(prompt string) []string{
	"claude-code": func(s string) []string {
		return []string{"claude", "-p", s, "--permission-mode", "acceptEdits",
			"--allowedTools", "Bash(aitk:*)", "Bash(git status:*)", "Bash(git diff:*)", "Bash(git log:*)"}
	},
	"codex":      func(s string) []string { return []string{"codex", "exec", "--sandbox", "workspace-write", s} },
	"opencode":   func(s string) []string { return []string{"opencode", "run", s} },
	"gemini-cli": func(s string) []string { return []string{"gemini", "-p", s} },
}

// RunInput configures aitk run; zero values fall back to [run] in aitk.toml, then to defaults.
type RunInput struct {
	Agent       string
	Cmd         string
	MaxTasks    int
	MaxAttempts int
	Timeout     time.Duration
	Goal        string
	Tag         string
	DryRun      bool
	Commit      bool
}

// RunTask is the outcome of one task in a run.
type RunTask struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Attempts int    `json:"attempts"`
	Status   string `json:"status"`            // the task's status afterwards
	Note     string `json:"note,omitempty"`    // why it stopped there
	Handoff  string `json:"handoff,omitempty"` // written by aitk run when the agent did not finish
	Commit   string `json:"commit,omitempty"`  // with --commit: the commit of a finished task
	// AgentError: the agent could not run at all; the task is left as it was.
	AgentError bool `json:"agent_error,omitempty"`
}

// agentStartupWindow: a failing agent that exits this fast without changing files never started working.
var agentStartupWindow = 20 * time.Second

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " / ")
}

// RunResult reports a run.
type RunResult struct {
	Agent   string      `json:"agent"`
	DryRun  bool        `json:"dry_run"`
	Planned []Candidate `json:"planned,omitempty"` // dry run: the order tasks would be taken in
	Tasks   []RunTask   `json:"tasks"`
	Stopped string      `json:"stopped"` // why the loop ended
}

// RunEnv marks processes started by aitk run, so an agent cannot start a nested run.
const RunEnv = "AITK_RUN"

// Run works through ready tasks one at a time with a headless agent. aitk never marks work done
// itself: each task finishes only through the agent's own `aitk close` and the Definition of Done.
// Tasks that need a person (review or UAT) are left in_review; tasks the agent cannot finish are
// handed over as blocked with a handoff. The loop stops after two unfinished tasks in a row.
func Run(p *project.Project, in RunInput, log io.Writer) (*RunResult, error) {
	if os.Getenv(RunEnv) != "" {
		return nil, apperr.New("E_RUN_NESTED", apperr.ExitGate, "finish the current task with aitk close",
			"aitk run cannot be started by an agent that aitk run started")
	}
	cfg := p.Config.RunSettings()
	agent, cmd := normalizeTool(firstNonEmpty(in.Agent, cfg.Agent)), firstNonEmpty(in.Cmd, cfg.Cmd)
	if in.Agent != "" && in.Cmd == "" {
		cmd = "" // an explicit --agent wins over a configured command
	}
	maxTasks := firstPositive(in.MaxTasks, cfg.MaxTasks, 10)
	maxAttempts := firstPositive(in.MaxAttempts, cfg.MaxAttempts, 2)
	timeout := in.Timeout
	if timeout <= 0 {
		timeout = time.Duration(firstPositive(cfg.TimeoutMinutes, 60)) * time.Minute
	}
	res := &RunResult{Agent: agent, DryRun: in.DryRun, Tasks: []RunTask{}}
	if cmd != "" {
		res.Agent = "custom"
	} else if _, ok := headless[agent]; !ok {
		return nil, apperr.New("E_USAGE", apperr.ExitUsage, "aitk run --agent claude-code|codex|opencode|gemini-cli (or --cmd \"<command>\", or [run] in aitk.toml)",
			"no agent to run (got %q)", agent)
	} else if _, err := exec.LookPath(headless[agent]("")[0]); err != nil && !in.DryRun {
		return nil, apperr.New("E_USAGE", apperr.ExitUsage, "install it, or pick another with --agent", "%s is not installed", headless[agent]("")[0])
	}
	filter := NextFilter{Goal: in.Goal, Tag: in.Tag, Skip: map[string]bool{}}
	if in.DryRun {
		nx := Next(p, filter)
		res.Planned = nx.Ready
		if len(res.Planned) > maxTasks {
			res.Planned = res.Planned[:maxTasks]
		}
		res.Stopped = "dry run"
		return res, nil
	}
	unfinished := 0
	for len(res.Tasks) < maxTasks {
		nx := Next(p, filter)
		if len(nx.Ready) == 0 {
			res.Stopped = "no ready tasks"
			if len(nx.Waiting) > 0 {
				res.Stopped += fmt.Sprintf(" (%d waiting on dependencies or other worktrees)", len(nx.Waiting))
			}
			break
		}
		c := nx.Ready[0]
		filter.Skip[c.ID] = true
		rt := runOne(p, c, agent, cmd, maxAttempts, timeout, log)
		if rt.AgentError {
			res.Tasks = append(res.Tasks, rt)
			res.Stopped = "the agent failed to start: fix the agent command or its login, then run again (" + rt.ID + " stays " + rt.Status + ")"
			break
		}
		if rt.Status == task.Done && (in.Commit || cfg.Commit) {
			sha, err := commitTask(p, rt.ID, res.Agent)
			if err != nil {
				rt.Note = "finished, but the commit failed: " + err.Error()
				res.Tasks = append(res.Tasks, rt)
				res.Stopped = "commit failed: fix it (hooks, identity, conflicts), commit, then run again"
				break
			}
			rt.Commit = sha
		}
		res.Tasks = append(res.Tasks, rt)
		if rt.Status == task.Done || rt.Status == task.InReview {
			unfinished = 0
			continue
		}
		if unfinished++; unfinished >= 2 {
			res.Stopped = "two tasks in a row were not finished: read their handoffs before running again"
			break
		}
	}
	if res.Stopped == "" {
		res.Stopped = fmt.Sprintf("reached the limit of %d tasks", maxTasks)
	}
	return res, nil
}

func runOne(p *project.Project, c Candidate, agent, cmd string, maxAttempts int, timeout time.Duration, log io.Writer) RunTask {
	rt := RunTask{ID: c.ID, Title: c.Title}
	t, err := TaskStart(p, c.ID)
	if err != nil {
		rt.Status, rt.Note = c.Status, "could not start: "+err.Error()
		return rt
	}
	for rt.Attempts < maxAttempts {
		rt.Attempts++
		fmt.Fprintf(log, "aitk run: %s %q, attempt %d of %d\n", t.ID, t.Title, rt.Attempts, maxAttempts)
		prompt := runPrompt(p, t, rt.Attempts, maxAttempts)
		before := profile.Fingerprint(p)
		ar := runAgent(p, agent, cmd, prompt, t.ID, timeout, log)
		code, runErr := ar.code, ar.err
		if cur, err := task.Find(p, t.ID); err == nil {
			t = cur
		}
		// An agent that fails at once without touching anything is broken (not installed, bad
		// flags, not logged in): stop the run instead of blaming the task.
		if (code != 0 || runErr != nil) && ar.duration < agentStartupWindow && t.Status == task.InProgress && profile.Fingerprint(p) == before {
			rt.Status, rt.AgentError = t.Status, true
			rt.Note = fmt.Sprintf("the agent failed to start (exit %d)", code)
			if runErr != nil {
				rt.Note = "the agent failed to start: " + runErr.Error()
			}
			if ar.tail != "" {
				rt.Note += ": " + lastLines(ar.tail, 3)
			}
			return rt
		}
		switch t.Status {
		case task.Done:
			rt.Status = task.Done
			return rt
		case task.InReview:
			rt.Status, rt.Note = task.InReview, "waiting for review passes or a person's acceptance (aitk uat accept "+t.ID+")"
			return rt
		case task.Blocked, task.Cancelled:
			rt.Status, rt.Note = t.Status, "the agent stopped the task; see its handoff"
			return rt
		}
		switch {
		case runErr != nil:
			rt.Note = runErr.Error()
		case code != 0:
			rt.Note = fmt.Sprintf("agent exited with %d", code)
		default:
			rt.Note = "agent finished without closing the task"
		}
		if t.Evidence != nil && t.Evidence.CheckFailures >= 2 {
			rt.Note += "; the check failed repeatedly"
			break
		}
	}
	// Not finished: hand it over as blocked so the next agent or person starts from a record.
	if s := session.Load(p); s.Active() != t.ID {
		TaskStart(p, t.ID)
	}
	summary := fmt.Sprintf("aitk run: %s did not finish this task after %d attempt(s): %s.", firstNonEmpty(agent, "the agent"), rt.Attempts, rt.Note)
	if cr, err := Close(p, CloseInput{Summary: summary, Knowledge: "none", Status: task.Blocked,
		Next: []string{"Read this task's handoffs and the last check output, then continue or split the task"}}); err == nil {
		rt.Handoff = cr.Handoff
	}
	rt.Status = task.Blocked
	return rt
}

// commitTask commits everything in the worktree as one Conventional Commit for a finished task.
func commitTask(p *project.Project, id, agent string) (string, error) {
	t, err := task.Find(p, id)
	if err != nil {
		return "", err
	}
	if out, _ := gitx.RunRaw(p.Root, "status", "--porcelain"); strings.TrimSpace(string(out)) == "" {
		return "", nil // the agent committed already
	}
	typ := "feat"
	for _, tag := range t.Tags {
		switch tag {
		case "bug", "fix", "security", "dependencies":
			typ = "fix"
		case "docs":
			typ = "docs"
		}
	}
	title := t.Title
	if typ == "fix" && strings.HasPrefix(strings.ToLower(title), "fix ") {
		title = strings.TrimSpace(title[4:]) // "Fix x" → "fix: x", not "fix: fix x"
	}
	r := []rune(title)
	if len(r) > 0 {
		r[0] = unicode.ToLower(r[0])
	}
	subject := textx.Truncate(typ+": "+string(r), 72)
	if _, err := gitx.RunRaw(p.Root, "add", "-A"); err != nil {
		return "", err
	}
	c := exec.Command("git", "commit", "-q", "-m", subject, "-m", "AI-Task: "+t.ID+"\nAI-Tool: "+agent)
	c.Dir = p.Root
	if out, err := c.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(checkSummary(string(out))))
	}
	out, err := gitx.RunRaw(p.Root, "rev-parse", "--short", "HEAD")
	return strings.TrimSpace(string(out)), err
}

func checkSummary(s string) string {
	if r := []rune(s); len(r) > 600 {
		return "…" + string(r[len(r)-600:])
	}
	return s
}

func runPrompt(p *project.Project, t *task.Task, attempt, max int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are working autonomously on one aitk task in this repository. No person is watching: do not ask questions.\n\n")
	fmt.Fprintf(&b, "Task: %s %s (file: %s). It is already started; do not start, create, or close other tasks.\n", t.ID, t.Title, t.File)
	if attempt > 1 {
		fmt.Fprintf(&b, "This is attempt %d of %d: an earlier attempt did not finish. Check the task file, the last handoff, and `aitk check` output first.\n", attempt, max)
	}
	b.WriteString(`
Steps:
1. Read the brief below and the task file. Keep the change focused on this task.
2. Implement it, then run ` + "`aitk check`" + ` until it passes. Mark each satisfied criterion: ` + "`aitk task update " + t.ID + " --ac-done N`" + `.
3. If the task is tagged bug, add a regression test that fails without the fix. Before closing, run ` + "`aitk impact`" + ` and make sure the code that depends on what you changed is covered by the check.
4. Finish with ` + "`aitk close --summary \"<what changed>\" --knowledge \"<lasting finding>|none\"`" + `. If close refuses, do what its fix line says and close again.
5. If you cannot finish (missing access, unclear requirement, the check fails twice for the same reason), run ` + "`aitk close --status blocked --summary \"<why>\" --knowledge none`" + ` and stop.
Never mark work done any other way, never edit aitk's evidence by hand, and never accept UAT.

`)
	b.WriteString(brief.Build(p, 0, "").Markdown)
	return b.String()
}

// agentRun is the outcome of one agent process.
type agentRun struct {
	code     int
	err      error
	duration time.Duration
	tail     string // last output, for diagnosing an agent that failed to start
}

func runAgent(p *project.Project, agent, cmd, prompt, id string, timeout time.Duration, log io.Writer) agentRun {
	pf := filepath.Join(p.AitkDir(), "run-prompt.md")
	if err := fsx.WriteFile(pf, []byte(prompt), 0o600); err != nil {
		return agentRun{err: err}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var c *exec.Cmd
	switch {
	case cmd != "" && runtime.GOOS == "windows":
		c = exec.CommandContext(ctx, "cmd", "/C", cmd)
	case cmd != "":
		c = exec.CommandContext(ctx, "sh", "-c", cmd)
	default:
		argv := append(headless[agent](prompt), p.Config.RunSettings().Args...)
		c = exec.CommandContext(ctx, argv[0], argv[1:]...)
	}
	c.Dir = p.Root
	c.Env = append(os.Environ(), RunEnv+"=1", "AITK_RUN_TASK="+id, "AITK_RUN_PROMPT_FILE="+pf)
	if cmd != "" {
		c.Stdin = strings.NewReader(prompt)
	}
	tail := &tailBuffer{max: 2000}
	c.Stdout, c.Stderr = io.MultiWriter(log, tail), io.MultiWriter(log, tail)
	c.WaitDelay = 10 * time.Second
	start := time.Now()
	err := c.Run()
	r := agentRun{duration: time.Since(start), tail: strings.TrimSpace(string(tail.b))}
	var ee *exec.ExitError
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		r.code, r.err = 124, fmt.Errorf("agent timed out after %s", timeout)
	case errors.As(err, &ee):
		r.code = ee.ExitCode()
	default:
		r.err = err
	}
	return r
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	b   []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = t.b[len(t.b)-t.max:]
	}
	return len(p), nil
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return strings.TrimSpace(x)
		}
	}
	return ""
}

func firstPositive(xs ...int) int {
	for _, x := range xs {
		if x > 0 {
			return x
		}
	}
	return 0
}
