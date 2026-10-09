package ops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/innaka-tech/ai-toolkit/v2/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/v2/internal/audit"
	"github.com/innaka-tech/ai-toolkit/v2/internal/brief"
	"github.com/innaka-tech/ai-toolkit/v2/internal/checkrun"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/profile"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/session"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
	"github.com/innaka-tech/ai-toolkit/v2/internal/textx"
)

// headless builds argv that runs a tool non-interactively. The prompt itself is in a file;
// the argument only points to it, so no shell or .cmd shim ever parses the brief's text.
// The defaults let the agent edit files, run aitk, and read git; anything more is the user's
// choice ([run].args, inserted before the prompt).
var headless = map[string]func(args []string, prompt string) []string{
	"claude-code": func(a []string, s string) []string {
		return append(append([]string{"claude"}, a...), "--permission-mode", "acceptEdits",
			"--allowedTools", "Read,Edit,Write,Glob,Grep,Bash(aitk:*),Bash(git status:*),Bash(git diff:*),Bash(git log:*)", "-p", s)
	},
	"codex": func(a []string, s string) []string {
		return append(append([]string{"codex", "exec"}, a...), "--sandbox", "workspace-write", s)
	},
	"opencode": func(a []string, s string) []string { return append(append([]string{"opencode", "run"}, a...), s) },
	"gemini-cli": func(a []string, s string) []string {
		return append(append([]string{"gemini"}, a...), "--approval-mode", "auto_edit", "-p", s)
	},
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
	Commit      *bool // nil: [run].commit
}

// RunTask is the outcome of one task in a run.
type RunTask struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Attempts int    `json:"attempts"`
	Status   string `json:"status"`            // the task's status afterwards
	Note     string `json:"note,omitempty"`    // why it stopped there
	Handoff  string `json:"handoff,omitempty"` // written by aitk run when the agent did not finish
	Commit   string `json:"commit,omitempty"`  // with --commit: the commit of the task's work
	Stash    string `json:"stash,omitempty"`   // with --commit: unfinished work set aside with git stash
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
	var args []string // [run].args belong to the configured agent only
	if in.Agent == "" || normalizeTool(in.Agent) == normalizeTool(cfg.Agent) {
		args = cfg.Args
	}
	maxTasks := firstPositive(in.MaxTasks, cfg.MaxTasks, 10)
	maxAttempts := firstPositive(in.MaxAttempts, cfg.MaxAttempts, 2)
	timeout := in.Timeout
	if timeout <= 0 {
		timeout = time.Duration(firstPositive(cfg.TimeoutMinutes, 60)) * time.Minute
	}
	commit := cfg.Commit
	if in.Commit != nil {
		commit = *in.Commit
	}
	res := &RunResult{Agent: agent, DryRun: in.DryRun, Tasks: []RunTask{}}
	if cmd != "" {
		res.Agent = "custom"
	} else if _, ok := headless[agent]; !ok {
		return nil, apperr.New("E_USAGE", apperr.ExitUsage, "aitk run --agent claude-code|codex|opencode|gemini-cli (or --cmd \"<command>\", or [run] in aitk.toml)",
			"no agent to run (got %q)", agent)
	} else if bin := headless[agent](nil, "")[0]; !in.DryRun {
		if _, err := exec.LookPath(bin); err != nil {
			return nil, apperr.New("E_USAGE", apperr.ExitUsage, "install it, or pick another with --agent", "%s is not installed", bin)
		}
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
	if commit {
		if dirty := worktreeChanges(p); len(dirty) > 0 {
			return nil, apperr.New("E_RUN_DIRTY", apperr.ExitGate, "commit or stash your changes first (git status), or run without --commit",
				"--commit needs a clean working tree so each commit holds one task's work; uncommitted: %s", strings.Join(firstN(dirty, 5), ", "))
		}
	}
	unlock, err := runLock(p)
	if err != nil {
		return nil, err
	}
	defer unlock()
	r := &runner{p: p, agent: agent, cmd: cmd, args: args, timeout: timeout, log: log, attempts: maxAttempts}
	if cmd != "" {
		r.agentName = "custom"
	} else {
		r.agentName = agent
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
		rt := r.one(c)
		if rt.AgentError {
			res.Tasks = append(res.Tasks, rt)
			res.Stopped = "the agent failed to start: fix the agent command or its login, then run again (" + rt.ID + " stays " + rt.Status + ")"
			break
		}
		if commit {
			if err := r.commitWork(&rt); err != nil {
				rt.Note = strings.TrimSpace(rt.Note + "; the commit failed: " + err.Error())
				res.Tasks = append(res.Tasks, rt)
				res.Stopped = "commit failed: fix it (hooks, identity, conflicts), commit, then run again"
				break
			}
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

type runner struct {
	p                *project.Project
	agent, agentName string
	cmd              string
	args             []string
	timeout          time.Duration
	attempts         int
	log              io.Writer
}

func (r *runner) one(c Candidate) RunTask {
	p := r.p
	rt := RunTask{ID: c.ID, Title: c.Title}
	t, err := TaskStart(p, c.ID)
	if err != nil {
		rt.Status, rt.Note = c.Status, "could not start: "+err.Error()
		return rt
	}
	for rt.Attempts < r.attempts {
		rt.Attempts++
		fmt.Fprintf(r.log, "aitk run: %s %q, attempt %d of %d\n", t.ID, t.Title, rt.Attempts, r.attempts)
		// Every attempt starts with the task active and claimed (a reopened task lost both).
		if cur, err := TaskStart(p, t.ID); err == nil {
			t = cur
		}
		snap := protectedOf(t)
		prompt := runPrompt(p, t, rt.Attempts, r.attempts)
		before := profile.Fingerprint(p)
		ar := r.runAgent(prompt, t.ID)
		code, runErr := ar.code, ar.err
		cur, ferr := task.Find(p, t.ID)
		if ferr != nil {
			rt.Status, rt.Note = "missing", "the task file disappeared during the attempt"
			return rt
		}
		t = cur
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
		case task.Done, task.InReview:
			claimed := t.Status
			target, why := r.decide(t, snap)
			if why != "" {
				t.Status, t.Closed, t.Updated = task.InProgress, "", task.Now()
				WithLock(p, func() error { return task.Save(p, t) })
				rt.Note = "the agent reported " + claimed + ", but aitk's own verification failed (" + why + "); reopened"
				continue
			}
			rt.Status = target
			if target == task.InReview {
				rt.Note = "waiting for review passes or a person's acceptance (aitk uat accept " + t.ID + ")"
			}
			return rt
		case task.Blocked, task.Cancelled:
			restoreProtected(t, snap) // whatever else the agent wrote, it cannot grant itself UAT or reviews
			WithLock(p, func() error { return task.Save(p, t) })
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
	return r.handOver(t, rt)
}

// protected holds the fields an agent must not be able to grant itself: a person's acceptance,
// review passes (which count only from other tools), audit results, and regression evidence.
type protected struct {
	uat        *task.UAT
	review     *task.Review
	audit      *task.Audit
	regression []string
}

func protectedOf(t *task.Task) protected {
	var s protected
	if t.Review != nil {
		r := *t.Review
		r.Passes = append([]task.Pass(nil), t.Review.Passes...)
		s.review = &r
	}
	if e := t.Evidence; e != nil {
		if e.UAT != nil {
			u := *e.UAT
			s.uat = &u
		}
		if e.Audit != nil {
			a := *e.Audit
			s.audit = &a
		}
		s.regression = append([]string(nil), e.RegressionTests...)
	}
	return s
}

func restoreProtected(t *task.Task, s protected) {
	t.Review = s.review
	if t.Evidence == nil {
		t.Evidence = &task.Evidence{}
	}
	t.Evidence.UAT, t.Evidence.Audit, t.Evidence.RegressionTests = s.uat, s.audit, s.regression
}

// decide re-derives the outcome of a task the agent reports as done or in_review, from what aitk
// itself can establish rather than from files the agent can write: the protected fields go back to
// their values before the attempt, the check (and the audit, when required) are run by aitk, the
// regression evidence is recomputed, and the Definition of Done decides. It returns the status to
// record, or why the task cannot leave in_progress.
func (r *runner) decide(t *task.Task, snap protected) (string, string) {
	p := r.p
	restoreProtected(t, snap)
	t.Evidence.RegressionTests = nil
	tree := profile.Fingerprint(p)
	if cmd := p.Config.Check.Cmd; cmd != "" {
		timeout, err := time.ParseDuration(p.Config.CheckTimeout())
		if err != nil {
			timeout = 10 * time.Minute
		}
		res := checkrun.Run(p.Root, cmd, timeout, false)
		if res.ExitCode != 0 {
			return "", fmt.Sprintf("the check fails when aitk runs it (exit %d)", res.ExitCode)
		}
		t.Evidence.Check = &task.Check{Cmd: cmd, ExitCode: 0, DurationMs: res.DurationMs, Summary: checkrun.Summary(strings.TrimSpace(res.Output), 1500), At: task.Now(), Tree: tree}
	}
	prof := t.Profile
	if computed := profile.Compute(p, profile.Diff(p)); !t.ProfilePinned || computed == "strict" {
		prof = computed
	}
	if p.Config.AuditRequired(prof) {
		a, _ := audit.Run(p, false)
		a.Tree = tree
		t.Evidence.Audit = a
		if !a.Passed {
			return "", "the security audit fails when aitk runs it"
		}
	}
	target, err := dod(p, t, prof, CloseInput{})
	if err != nil {
		return "", "the Definition of Done does not hold: " + err.Error()
	}
	if IsBugTask(t) {
		t.Evidence.RegressionTests = capList(regressionTests(p, t), 10)
	}
	t.Status, t.Profile, t.Updated = target, prof, task.Now()
	if target == task.Done && t.Closed == "" {
		t.Closed = t.Updated
	}
	if target != task.Done {
		t.Closed = ""
	}
	if err := WithLock(p, func() error { return task.Save(p, t) }); err != nil {
		return "", err.Error()
	}
	return target, ""
}

// handOver closes an unfinished task as blocked with a handoff, attributed to the agent.
func (r *runner) handOver(t *task.Task, rt RunTask) RunTask {
	p := r.p
	defer setEnv("AITK_TOOL", r.agentName)()
	if s := session.Load(p); s.Active() != t.ID {
		if _, err := TaskStart(p, t.ID); err != nil {
			rt.Status, rt.Note = t.Status, strings.TrimSpace(rt.Note+"; could not hand it over: "+err.Error())
			return rt
		}
	}
	summary := fmt.Sprintf("aitk run: %s did not finish this task after %d attempt(s): %s.", r.agentName, rt.Attempts, rt.Note)
	cr, err := Close(p, CloseInput{Summary: summary, Knowledge: "none", Status: task.Blocked,
		Next: []string{"Read this task's handoffs and the last check output, then continue or split the task"}})
	switch {
	case err != nil:
		rt.Status, rt.Note = t.Status, strings.TrimSpace(rt.Note+"; could not hand it over: "+err.Error())
	case !strings.EqualFold(cr.Task, t.ID):
		rt.Status, rt.Note = t.Status, strings.TrimSpace(rt.Note+"; the handover closed "+cr.Task+" instead")
	default:
		rt.Status, rt.Handoff = task.Blocked, cr.Handoff
	}
	return rt
}

// setEnv sets an environment variable and returns a function that restores it.
func setEnv(k, v string) func() {
	old, had := os.LookupEnv(k)
	os.Setenv(k, v)
	return func() {
		if had {
			os.Setenv(k, old)
		} else {
			os.Unsetenv(k)
		}
	}
}

// worktreeChanges lists changed and untracked paths.
func worktreeChanges(p *project.Project) []string {
	out, _ := gitx.RunRaw(p.Root, "status", "--porcelain", "-z", "--untracked-files=all")
	var paths []string
	toks := strings.Split(string(out), "\x00")
	for i := 0; i < len(toks); i++ {
		e := toks[i]
		if len(e) < 4 {
			continue
		}
		paths = append(paths, e[3:])
		if (e[0] == 'R' || e[0] == 'C') && i+1 < len(toks) {
			i++
			paths = append(paths, toks[i]) // the rename's source: part of the same change
		}
	}
	return paths
}

func firstN(xs []string, n int) []string {
	if len(xs) > n {
		return append(xs[:n:n], "…")
	}
	return xs
}

// isAitkState reports whether a path is aitk's own state (tasks, handoffs, knowledge, indexes).
func isAitkState(path string) bool {
	return strings.HasPrefix(path, project.DocsAI+"/") || path == "ai-state.json"
}

// commitWork commits the task's work. The tree was clean when the run started and every task
// leaves it clean, so the changes belong to this task. Finished work (done or in_review) is one
// Conventional Commit; unfinished work is set aside with git stash and only aitk's handover
// record is committed, so a broken attempt never lands in the history or in the next task's commit.
func (r *runner) commitWork(rt *RunTask) error {
	p := r.p
	if len(worktreeChanges(p)) == 0 {
		return nil
	}
	t, err := task.Find(p, rt.ID)
	if err != nil {
		return err
	}
	finished := rt.Status == task.Done || rt.Status == task.InReview
	if finished {
		if _, err := literalGit(p, "add", "-A"); err != nil {
			return err
		}
		if err := r.commit(rt, commitSubject(t)); err != nil {
			return err
		}
	} else {
		// Commit aitk's handover record alone, then set everything else aside. Unstaging first
		// means a rename or a staged change by the agent cannot ride along in the commit; stashing
		// without pathspecs keeps renames and odd file names intact.
		if _, err := literalGit(p, "reset", "-q"); err != nil {
			return err
		}
		if _, err := literalGit(p, "add", "-A", "--", project.DocsAI, "ai-state.json"); err != nil {
			return err
		}
		if out, _ := literalGit(p, "diff", "--cached", "--name-only"); strings.TrimSpace(out) != "" {
			if err := r.commit(rt, "chore(aitk): hand over "+t.ID); err != nil {
				return err
			}
		}
		if len(worktreeChanges(p)) > 0 {
			msg := "aitk run: unfinished work on " + t.ID
			// No pathspecs here, and not literalGit: stash uses pathspec magic internally.
			if _, err := gitx.RunRaw(p.Root, "stash", "push", "--include-untracked", "-m", msg); err != nil {
				return fmt.Errorf("could not set the unfinished work aside: %w", err)
			}
			rt.Stash = msg
		}
	}
	if left := worktreeChanges(p); len(left) > 0 {
		return fmt.Errorf("files left in the working tree: %s", strings.Join(firstN(left, 5), ", "))
	}
	return nil
}

func (r *runner) commit(rt *RunTask, subject string) error {
	if out, err := literalGit(r.p, "commit", "-q", "-m", subject, "-m", "AI-Task: "+rt.ID+"\nAI-Tool: "+r.agentName); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(checkSummary(out)))
	}
	out, err := gitx.RunRaw(r.p.Root, "rev-parse", "--short", "HEAD")
	rt.Commit = strings.TrimSpace(string(out))
	return err
}

// literalGit runs git with pathspec magic off, so a file named ":x" is just a file.
func literalGit(p *project.Project, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = p.Root
	c.Env = append(os.Environ(), "GIT_LITERAL_PATHSPECS=1")
	out, err := c.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func commitSubject(t *task.Task) string {
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
	rs := []rune(title)
	if len(rs) > 0 {
		rs[0] = unicode.ToLower(rs[0])
	}
	return textx.Truncate(typ+": "+string(rs), 72)
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

func (r *runner) runAgent(prompt, id string) agentRun {
	p := r.p
	pf := filepath.Join(p.AitkDir(), "run-prompt.md")
	if err := fsx.WriteFile(pf, []byte(prompt), 0o600); err != nil {
		return agentRun{err: err}
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	var c *exec.Cmd
	if r.cmd != "" {
		c = shellCommand(ctx, r.cmd)
		c.Stdin = strings.NewReader(prompt)
	} else {
		pointer := "Read the file " + pf + " and follow its instructions exactly. It describes the one aitk task to finish in this repository."
		argv := headless[r.agent](r.args, pointer)
		c = exec.CommandContext(ctx, argv[0], argv[1:]...)
	}
	killTree(c) // a timeout ends the agent and everything it started
	c.Dir = p.Root
	c.Env = append(os.Environ(), RunEnv+"=1", "AITK_TOOL="+r.agentName, "AITK_RUN_TASK="+id, "AITK_RUN_PROMPT_FILE="+pf)
	tail := &tailBuffer{max: 2000}
	c.Stdout, c.Stderr = io.MultiWriter(r.log, tail), io.MultiWriter(r.log, tail)
	c.WaitDelay = 10 * time.Second
	start := time.Now()
	err := c.Run()
	endTree(c) // background processes the agent left behind (servers, watchers) end with it
	res := agentRun{duration: time.Since(start), tail: strings.TrimSpace(string(tail.b))}
	var ee *exec.ExitError
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.code, res.err = 124, fmt.Errorf("agent timed out after %s", r.timeout)
	case errors.As(err, &ee):
		res.code = ee.ExitCode()
	default:
		res.err = err
	}
	return res
}

// runLock allows one aitk run per worktree. The kernel holds the lock and releases it when the
// process ends, so a crash leaves nothing stale.
func runLock(p *project.Project) (func(), error) {
	path := filepath.Join(p.AitkDir(), "run.lock")
	l, err := fsx.Acquire(path, 0)
	if errors.Is(err, fsx.ErrLocked) {
		return nil, apperr.New("E_LOCKED", apperr.ExitConflict, "wait for it to finish, or use another worktree (aitk work <id>)",
			"another aitk run is working in this worktree")
	}
	if err != nil {
		return nil, err
	}
	return l.Release, nil
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
