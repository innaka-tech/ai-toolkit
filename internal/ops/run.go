package ops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/innaka-tech/ai-toolkit/v2/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/v2/internal/audit"
	"github.com/innaka-tech/ai-toolkit/v2/internal/brief"
	"github.com/innaka-tech/ai-toolkit/v2/internal/checkrun"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/handoff"
	"github.com/innaka-tech/ai-toolkit/v2/internal/proc"
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
		if r.guard != nil {
			r.guard.checkGitDirs(p)
			if len(r.guard.failed) > 0 { // in every mode: the next task must not start from a forged baseline
				rt.Note = strings.TrimPrefix(rt.Note+"; could not restore "+strings.Join(dedupe(r.guard.failed), ", "), "; ")
				res.Tasks = append(res.Tasks, rt)
				res.Stopped = "aitk could not restore what the agent changed outside its task: put those files back by hand (git status), then run again"
				break
			}
		}
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
	guard            *attemptGuard // the current task's guard, re-applied before verification and commits
}

func (r *runner) one(c Candidate) (rt RunTask) {
	p := r.p
	rt = RunTask{ID: c.ID, Title: c.Title}
	var tampered []string // what the agent changed outside its own task, restored by aitk
	defer func() {
		if len(tampered) > 0 {
			rt.Note = strings.TrimPrefix(rt.Note+"; restored what the agent changed outside its task: "+strings.Join(dedupe(tampered), ", "), "; ")
		}
	}()
	r.guard = nil
	t, err := TaskStart(p, c.ID)
	if err != nil {
		rt.Status, rt.Note = c.Status, "could not start: "+err.Error()
		return rt
	}
	// One guard for the whole task: a later attempt must not take an earlier attempt's
	// unrestorable change as its baseline.
	guard := guardOf(p, t, r.agentName)
	r.guard = guard
	for rt.Attempts < r.attempts {
		rt.Attempts++
		fmt.Fprintf(r.log, "aitk run: %s %q, attempt %d of %d\n", t.ID, t.Title, rt.Attempts, r.attempts)
		// Every attempt starts with the task active and claimed (a reopened task lost both).
		if cur, err := TaskStart(p, t.ID); err == nil {
			t = cur
		}
		snap := protectedOf(t)
		started := task.Now()
		prompt := runPrompt(p, t, rt.Attempts, r.attempts)
		if rt.Attempts > 1 && rt.Note != "" {
			prompt = "The previous attempt did not finish: " + rt.Note + "\n\n" + prompt
		}
		before := profile.Fingerprint(p)
		ar := r.runAgent(prompt, t.ID)
		tampered = append(tampered, guard.restore(p)...) // before verification and any commit
		code, runErr := ar.code, ar.err
		cur, ferr := task.Find(p, t.ID)
		if ferr != nil {
			rt.Status, rt.Note = "missing", "the task file disappeared during the attempt"
			return rt
		}
		t = cur
		if len(guard.failed) > 0 {
			return r.guardFailed(t, snap, rt)
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
		case task.Done, task.InReview:
			claimed := t.Status
			target, why := r.decide(t, snap, started)
			tampered = append(tampered, guard.restore(p)...) // the check ran project code: look again
			if len(guard.failed) > 0 {
				return r.guardFailed(t, snap, rt)
			}
			if why != "" {
				t.Status, t.Closed, t.Updated = task.InProgress, "", task.Now()
				WithLock(p, func() error { return task.Save(p, t) })
				rt.Note = "the agent reported " + claimed + ", but aitk's own verification failed (" + why + "); reopened"
				continue
			}
			rt.Status, rt.Note = target, "" // an earlier attempt's note no longer applies
			if target == task.InReview {
				rt.Note = "waiting for a person's acceptance (aitk uat accept " + t.ID + ")"
				kept := t.Review
				t.Review = snap.review // the same test decide used: passes from outside the attempt
				independent := reviewDone(t)
				t.Review = kept
				if t.Profile == "strict" && (!reviewDone(t) || !independent) {
					rt.Note = "waiting for an independent review pass by another tool (aitk review pass " + t.ID + " --findings N)"
				}
			}
			return rt
		case task.Blocked, task.Cancelled:
			r.restore(t, snap) // whatever else the agent wrote, it cannot grant itself UAT or reviews
			WithLock(p, func() error { return task.Save(p, t) })
			rt.Status, rt.Note = t.Status, "the agent stopped the task; see its handoff"
			return rt
		}
		// Still unfinished: whatever the agent wrote, it cannot keep grants it gave itself.
		r.restore(t, snap)
		WithLock(p, func() error { return task.Save(p, t) })
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

// guardFailed ends the task when the guard could not put something back: nothing the agent
// did can be verified, so the task stays open and keeps no grants. Run then stops.
func (r *runner) guardFailed(t *task.Task, snap protected, rt RunTask) RunTask {
	r.restore(t, snap)
	if t.Status == task.Done || t.Status == task.InReview {
		t.Status, t.Closed = task.InProgress, ""
	}
	t.Updated = task.Now()
	WithLock(r.p, func() error { return task.Save(r.p, t) })
	rt.Status = t.Status
	rt.Note = "stopped: could not restore what the agent changed outside its task"
	return rt
}

// protected holds the fields an agent must not be able to grant itself: a person's acceptance,
// review passes (which count only from other tools), audit results, and regression evidence.
type protected struct {
	uat        *task.UAT
	review     *task.Review
	audit      *task.Audit
	regression []string
	// What decides which gates apply: an agent must not lower its own bar.
	profile   string
	pinned    bool
	tags      []string
	goal      string
	dependsOn []string
	criteria  []string // texts; marking them done is the agent's job, rewording them is not
	// Who worked on the task decides which review passes count as independent.
	createdBy string
	workers   []string
}

func protectedOf(t *task.Task) protected {
	s := protected{profile: t.Profile, pinned: t.ProfilePinned, tags: append([]string(nil), t.Tags...),
		goal: t.Goal, dependsOn: append([]string(nil), t.DependsOn...),
		createdBy: t.CreatedBy, workers: append([]string(nil), t.Workers...)}
	for _, c := range t.Criteria() {
		s.criteria = append(s.criteria, c.Text)
	}
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

// restore puts back what the agent may not change and records the agent as a worker: aitk does
// this on every path an attempt can end on, so the agent's own passes are never independent.
func (r *runner) restore(t *task.Task, snap protected) {
	restoreProtected(t, snap)
	t.AddWorker(r.agentName)
}

// attemptGuard holds what an agent must not change during an attempt: aitk.toml, the other
// tasks' files, the identity of its own task file, and the git configuration aitk's own git
// commands read afterwards. New task files are allowed (a persona may file follow-up tasks), but
// only as new work: a file that claims an existing task's id (or no valid id) is removed, and a
// new task keeps only its description. It fails closed: what it cannot restore stops the commit.
type attemptGuard struct {
	files   map[string][]byte // absolute path -> content before the attempt
	present map[string]bool   // absolute path -> existed before the attempt
	ids     []string          // ids of the tasks that existed before the attempt
	curID   string
	curFile string // absolute
	curWas  []byte
	agent   string
	failed  []string // what could not be restored
	// Where git must still resolve to before a commit.
	gitDir, commonDir string
	realRoot          string            // the repository root with symlinks resolved
	layout            map[string]string // existing parts of docs/ai/tasks -> where they resolved
	gitWasDir         bool              // .git was a directory (not a gitdir: file)
	config            string            // the repository's effective config and branch, see repoConfig
	accepted          map[string]string // new task files kept so far (path -> id): not duplicates of themselves
}

func guardOf(p *project.Project, t *task.Task, agent string) *attemptGuard {
	g := &attemptGuard{files: map[string][]byte{}, present: map[string]bool{}, ids: []string{t.ID}, curID: t.ID,
		curFile: p.Path(filepath.ToSlash(t.File)), agent: agent}
	g.curWas, _ = os.ReadFile(g.curFile)
	keep := func(abs string) {
		b, err := os.ReadFile(abs)
		g.present[abs] = err == nil
		if err == nil {
			g.files[abs] = b
		}
	}
	keep(p.Path(project.ConfigFile))
	common := p.CommonDir
	if common == "" {
		common = filepath.Join(p.Root, ".git")
	}
	for _, f := range []string{filepath.Join(common, "config"), filepath.Join(common, "info", "attributes"), filepath.Join(p.GitDir, "config.worktree"),
		filepath.Join(p.GitDir, "commondir"), filepath.Join(p.GitDir, "gitdir"), filepath.Join(p.Root, ".git")} {
		if fi, err := os.Lstat(f); err == nil && fi.IsDir() {
			continue // the .git directory itself: its contents are guarded file by file
		}
		keep(f)
	}
	for _, f := range includedConfigs(p) { // files the repository's config pulls in (include.path)
		keep(f)
	}
	g.gitDir, g.commonDir = p.GitDir, common
	g.realRoot, _ = filepath.EvalSymlinks(p.Root)
	g.layout = map[string]string{}
	for d := filepath.FromSlash(project.TasksDir); d != "." && d != string(filepath.Separator); d = filepath.Dir(d) {
		if got, err := filepath.EvalSymlinks(filepath.Join(p.Root, d)); err == nil {
			g.layout[d] = got
		}
	}
	g.config = repoConfig(p)
	if fi, err := os.Lstat(filepath.Join(p.Root, ".git")); err == nil && fi.IsDir() {
		g.gitWasDir = true
	}
	tasks, _ := task.List(p)
	for _, o := range tasks {
		g.ids = append(g.ids, o.ID)
		if abs := p.Path(filepath.ToSlash(o.File)); abs != g.curFile {
			keep(abs)
		}
	}
	return g
}

func (g *attemptGuard) knownID(id string) bool {
	for _, k := range g.ids {
		if strings.EqualFold(k, id) {
			return true
		}
	}
	return false
}

// restore puts the guarded state back and names what it changed. It runs under the project
// lock when it can, and without it when the lock is broken (an agent must not be able to switch
// the restore off by breaking the lock).
func (g *attemptGuard) restore(p *project.Project) []string {
	if g == nil {
		return nil
	}
	var changed []string
	ran := false
	work := func() error {
		ran = true
		changed = g.restoreLocked(p)
		return nil
	}
	if err := WithLock(p, work); err != nil && !ran {
		work()
		changed = append(changed, "(the project lock was unavailable)")
	}
	sort.Strings(changed)
	return changed
}

func (g *attemptGuard) put(abs string, want []byte) bool {
	os.RemoveAll(abs) // whatever sits there now (a directory, a symlink) goes first
	if err := fsx.WriteFile(abs, want, 0o644); err != nil {
		g.failed = append(g.failed, abs)
		return false
	}
	return true
}

func (g *attemptGuard) restoreLocked(p *project.Project) []string {
	var changed []string
	if moved := g.moved(p); moved != "" {
		g.failed = append(g.failed, moved)
		return nil // writing now would follow the agent's links out of the repository
	}
	for abs, want := range g.files {
		fi, lerr := os.Lstat(abs)
		regular := lerr == nil && fi.Mode().IsRegular()
		if got, err := os.ReadFile(abs); regular && err == nil && bytes.Equal(got, want) {
			continue
		}
		if g.put(abs, want) {
			changed = append(changed, relTo(p, abs))
		}
	}
	for abs, was := range g.present {
		if !was { // git config files the agent created
			if _, err := os.Lstat(abs); err == nil && os.RemoveAll(abs) == nil {
				changed = append(changed, relTo(p, abs)+" (removed)")
			}
		}
	}
	// The agent's own task file keeps its identity: a changed id (or a missing file) is forgery.
	if len(g.curWas) > 0 {
		if cur, err := task.Load(p, relTo(p, g.curFile)); err != nil || !strings.EqualFold(cur.ID, g.curID) {
			if g.put(g.curFile, g.curWas) {
				changed = append(changed, relTo(p, g.curFile)+" (its id or file was changed)")
			}
		}
	}
	entries, _ := os.ReadDir(p.Path(project.TasksDir))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		abs := p.Path(project.TasksDir, e.Name())
		if _, guarded := g.files[abs]; guarded || abs == g.curFile {
			continue
		}
		r := relTo(p, abs)
		nt, err := task.Load(p, r)
		switch {
		case err != nil || !task.IDPattern.MatchString(nt.ID):
			if os.Remove(abs) == nil {
				changed = append(changed, r+" (removed: not a valid task)")
			}
		case g.accepted[abs] != "" && strings.EqualFold(g.accepted[abs], nt.ID):
			// kept by an earlier restore: only reset it again below
			if sanitizeNewTask(nt, g.agent) && task.Save(p, nt) != nil && os.Remove(abs) == nil {
				changed = append(changed, r+" (removed: could not be reset to its description)")
			}
		case g.knownID(nt.ID):
			if os.Remove(abs) == nil {
				changed = append(changed, r+" (removed: duplicate of "+nt.ID+")")
			}
		default:
			g.ids = append(g.ids, nt.ID) // a second new file with this id is a duplicate too
			if g.accepted == nil {
				g.accepted = map[string]string{}
			}
			g.accepted[abs] = nt.ID
			if !sanitizeNewTask(nt, g.agent) {
				continue
			}
			if err := task.Save(p, nt); err != nil {
				if os.Remove(abs) == nil { // fail closed: an invalid seeded task is not kept as written
					changed = append(changed, r+" (removed: could not be reset to its description)")
				}
			} else {
				changed = append(changed, r+" (new task reset to its description)")
			}
		}
	}
	// Last, what git will actually read: an include the guard could not list (missing, empty,
	// conditional on a branch) or a branch switch shows up here, and fails closed.
	if repoConfig(p) != g.config {
		g.failed = append(g.failed, "git configuration (changed through an included file or a branch switch)")
	}
	return changed
}

// moved names the first part of the layout that no longer resolves inside the repository: any
// existing component of docs/ai/tasks that is (or sits under) a symlink or junction, and a .git
// directory that was replaced. Empty means the layout is intact.
func (g *attemptGuard) moved(p *project.Project) string {
	if g.realRoot == "" {
		return p.Root + " (could not resolve the repository root)"
	}
	for d := filepath.FromSlash(project.TasksDir); d != "." && d != string(filepath.Separator); d = filepath.Dir(d) {
		got, err := filepath.EvalSymlinks(filepath.Join(p.Root, d))
		if err != nil {
			if _, lerr := os.Lstat(filepath.Join(p.Root, d)); lerr != nil {
				continue // missing: restored as a plain directory below
			}
			return filepath.ToSlash(d) + " (cannot be resolved)"
		}
		if was, ok := g.layout[d]; ok && got != was {
			return filepath.ToSlash(d) + " (now resolves elsewhere: a link)"
		}
		if r, err := filepath.Rel(g.realRoot, got); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(d) + " (a link out of the repository)"
		}
	}
	if g.gitWasDir {
		fi, err := os.Lstat(filepath.Join(p.Root, ".git"))
		got, eerr := filepath.EvalSymlinks(filepath.Join(p.Root, ".git"))
		if err != nil || !fi.IsDir() || eerr != nil || got != filepath.Join(g.realRoot, ".git") {
			return ".git (no longer the repository's own directory)"
		}
	}
	return ""
}

// includedConfigs lists the files the repository's own config pulls in with include.path or
// includeIf, wherever they are (a tracked file, a file in .git, the main checkout of a linked
// worktree). Global and system config are not the repository's and are left alone.
func includedConfigs(p *project.Project) []string {
	seen := map[string]bool{}
	var files []string
	for _, scope := range []string{"--local", "--worktree"} {
		c := exec.Command("git", append(safeGitFlags(), "config", scope, "--list", "--show-origin", "--includes", "-z")...)
		c.Dir = p.Root
		out, err := c.Output()
		if err != nil {
			continue
		}
		parts := strings.Split(string(out), "\x00")
		for i := 0; i+1 < len(parts); i += 2 { // origin, then key and value
			f, ok := strings.CutPrefix(parts[i], "file:")
			if !ok {
				continue
			}
			if !filepath.IsAbs(f) {
				f = filepath.Join(p.Root, f)
			}
			if f = filepath.Clean(f); !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	return files
}

// repoConfig is what git reads for this repository beyond global and system config, includes
// resolved, plus the current branch (includeIf onbranch depends on it). Errors are part of it.
func repoConfig(p *project.Project) string {
	var b strings.Builder
	for _, args := range [][]string{{"config", "--local", "--list", "--includes", "-z"}, {"config", "--worktree", "--list", "--includes", "-z"}, {"symbolic-ref", "-q", "HEAD"}} {
		c := exec.Command("git", append(safeGitFlags(), args...)...)
		c.Dir = p.Root
		out, err := c.CombinedOutput()
		fmt.Fprintf(&b, "%s\x00%v\x00", out, err)
	}
	return b.String()
}

// checkGitDirs fails the guard if git would now resolve to another git directory (an agent
// pointing commondir or the .git file elsewhere, to bring its own config and filters).
func (g *attemptGuard) checkGitDirs(p *project.Project) {
	out, err := literalGit(p, "rev-parse", "--absolute-git-dir", "--git-common-dir")
	if err != nil {
		g.failed = append(g.failed, "git directory (rev-parse failed)")
		return
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	same := func(a, b string) bool {
		ra, _ := filepath.EvalSymlinks(a)
		rb, _ := filepath.EvalSymlinks(b)
		return ra != "" && ra == rb
	}
	if len(lines) < 2 {
		g.failed = append(g.failed, "git directory (unexpected rev-parse output)")
		return
	}
	common := lines[1]
	if !filepath.IsAbs(common) {
		common = filepath.Join(p.Root, common)
	}
	if !same(lines[0], g.gitDir) || !same(common, g.commonDir) {
		g.failed = append(g.failed, "git directory (points elsewhere)")
	}
}

func relTo(p *project.Project, abs string) string {
	if r, err := filepath.Rel(p.Root, abs); err == nil {
		return filepath.ToSlash(r)
	}
	return abs
}

// sanitizeNewTask keeps a task the agent filed as plain new work. It reports whether it changed.
func sanitizeNewTask(t *task.Task, agent string) bool {
	changed := t.Review != nil || t.Evidence != nil || len(t.Workers) > 0 || t.CreatedBy != agent ||
		(t.ProfilePinned && t.Profile != "strict") || (t.Status != task.Todo && t.Status != task.Blocked) || t.Closed != ""
	t.Review, t.Evidence, t.Workers, t.CreatedBy, t.Closed = nil, nil, nil, agent, ""
	if t.ProfilePinned && t.Profile != "strict" {
		t.ProfilePinned = false // pinning raises the bar only
	}
	if t.Status != task.Todo && t.Status != task.Blocked {
		t.Status = task.Todo
	}
	return changed
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// ownPasses is the protected review plus the passes the agent appended during the attempt,
// renumbered and attributed to the agent.
func ownPasses(base, attempt *task.Review, agent string) *task.Review {
	out := &task.Review{}
	if base != nil {
		out.Passes = append(out.Passes, base.Passes...)
	}
	if attempt != nil {
		for i := len(out.Passes); i < len(attempt.Passes); i++ {
			ps := attempt.Passes[i]
			ps.N, ps.By, ps.Session = len(out.Passes)+1, agent, ""
			out.Passes = append(out.Passes, ps)
		}
	}
	return out
}

func restoreProtected(t *task.Task, s protected) {
	t.Profile, t.ProfilePinned, t.Tags, t.Goal, t.DependsOn = s.profile, s.pinned, s.tags, s.goal, s.dependsOn
	t.CreatedBy = s.createdBy
	workers := append([]string(nil), s.workers...) // an agent can add itself but never remove anyone
	for _, w := range t.Workers {
		if !containsFold(workers, w) {
			workers = append(workers, w)
		}
	}
	t.Workers = workers
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
func (r *runner) decide(t *task.Task, snap protected, since string) (string, string) {
	p := r.p
	attempt := t.Review // the agent's own passes: they count for the non-strict review loop only
	r.restore(t, snap)
	cur := t.Criteria()
	if len(cur) < len(snap.criteria) {
		return "", "acceptance criteria were removed"
	}
	for i, text := range snap.criteria {
		if cur[i].Text != text {
			return "", fmt.Sprintf("acceptance criterion %d was reworded", i+1)
		}
	}
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
	if computed := profile.Compute(p, TaskChanges(p, t)); !t.ProfilePinned || computed == "strict" {
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
	// Only passes appended during this attempt are kept, attributed to the agent whatever the
	// file says. The agent is a worker, so for strict work they can never stand in for another
	// tool's independent review; they do count for the non-strict review loop, and they show
	// the agent did its own pass, so a strict task left in_review does not hold the worktree.
	t.Review = ownPasses(snap.review, attempt, r.agentName)
	target, err := dod(p, t, prof, CloseInput{})
	if err == nil && target == task.Done && prof == "strict" {
		// The independent review must stand on passes recorded outside this attempt: the agent's
		// own clean pass after another tool's findings is not that tool confirming the fix.
		kept := t.Review
		t.Review = snap.review
		independent := reviewDone(t)
		t.Review = kept
		if !independent {
			target = task.InReview
		}
	}
	if err != nil {
		t.Review = snap.review // a failed attempt keeps nothing the agent wrote about reviews
		return "", "the Definition of Done does not hold: " + err.Error()
	}
	if target == task.InReview && prof != "strict" && p.Config.ReviewRequired(prof) && !reviewSatisfied(p, t, prof) {
		t.Review = snap.review // the agent's own loop is unfinished: work for the next attempt
		return "", "review until clean is not finished (no clean review pass on the current code)"
	}
	if target == task.InReview && prof == "strict" && p.Config.ReviewLoop() && !reviewDone(t) && (t.Review == nil || len(t.Review.Passes) == 0 || t.Review.Passes[len(t.Review.Passes)-1].Findings > 0) {
		t.Review = snap.review // strict work waits for another tool only after the agent's own clean pass
		return "", "your own review pass is missing or found something to fix (strict work needs it before another tool's pass)"
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
	// Close writes the record of what was done; if the agent never ran it, aitk run writes one.
	recorded := false
	for _, h := range handoff.List(p) {
		if strings.EqualFold(h.Task, t.ID) && h.At >= since {
			recorded = true
			break
		}
	}
	if !recorded {
		outcome := map[string]string{task.Done: "done", task.InReview: "progress"}[target]
		handoff.Write(p, handoff.Meta{Task: t.ID, At: task.Now(), By: r.agentName, Outcome: outcome, TaskStatus: target, Check: t.Evidence.Check},
			"## "+t.Title+"\n\nVerified by aitk run (check, Definition of Done). The agent did not record a summary with aitk close.\n")
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
	out, _ := gitx.RunRaw(p.Root, append(safeGitFlags(), "status", "--porcelain", "-z", "--untracked-files=all")...)
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
	if again := r.guard.restore(p); len(again) > 0 { // anything still running could have changed them
		rt.Note = strings.TrimPrefix(rt.Note+"; restored before the commit: "+strings.Join(again, ", "), "; ")
	}
	if r.guard != nil {
		r.guard.checkGitDirs(p)
	}
	if r.guard != nil && len(r.guard.failed) > 0 {
		return fmt.Errorf("could not restore %s after the attempt; nothing committed", strings.Join(dedupe(r.guard.failed), ", "))
	}
	if len(worktreeChanges(p)) == 0 {
		return nil
	}
	t, err := task.Find(p, rt.ID)
	if err != nil {
		return err
	}
	finished := rt.Status == task.Done || rt.Status == task.InReview
	if finished {
		// The index is a file the agent can write (skip-worktree, assume-unchanged, staged
		// content): rebuild it from HEAD so only the working tree aitk restored is committed.
		if _, err := literalGit(p, "read-tree", "HEAD"); err != nil {
			return err
		}
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
		if _, err := literalGit(p, "read-tree", "HEAD"); err != nil { // also drops index flags
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
			if _, err := gitx.RunRaw(p.Root, append(safeGitFlags(), "stash", "push", "--include-untracked", "-m", msg)...); err != nil {
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
	// The repository's hooks are files the agent could have written: commit without them.
	noHooks, err := os.MkdirTemp("", "aitk-nohooks-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(noHooks)
	if out, err := literalGit(r.p, "-c", "core.hooksPath="+noHooks, "commit", "--no-verify", "-q", "-m", subject, "-m", "AI-Task: "+rt.ID+"\nAI-Tool: "+r.agentName); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(checkSummary(out)))
	}
	out, err := gitx.RunRaw(r.p.Root, "rev-parse", "--short", "HEAD")
	rt.Commit = strings.TrimSpace(string(out))
	return err
}

// safeGitFlags switch off git settings that run programs (an agent can write .git/config):
// the fsmonitor hook and repository hooks. The guard also restores .git/config after attempts.
func safeGitFlags() []string {
	return []string{"-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + os.DevNull}
}

// literalGit runs git with pathspec magic off, so a file named ":x" is just a file.
func literalGit(p *project.Project, args ...string) (string, error) {
	c := exec.Command("git", append(safeGitFlags(), args...)...)
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
3. If the task is tagged bug, add a regression test that fails without the fix. Before closing, run ` + "`aitk impact`" + ` and make sure the code that depends on what you changed is covered by the check.` + reviewStep(p, t) + `
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
		c = proc.Shell(ctx, r.cmd)
		c.Stdin = strings.NewReader(prompt)
	} else {
		pointer := "Read the file " + pf + " and follow its instructions exactly. It describes the one aitk task to finish in this repository."
		argv := headless[r.agent](r.args, pointer)
		c = exec.CommandContext(ctx, argv[0], argv[1:]...)
		c.Stdin = strings.NewReader("") // tools like codex exec read stdin to EOF when it is not a terminal
	}
	proc.KillTree(c) // a timeout ends the agent and everything it started
	c.Dir = p.Root
	c.Env = append(os.Environ(), RunEnv+"=1", "AITK_TOOL="+r.agentName, "AITK_RUN_TASK="+id, "AITK_RUN_PROMPT_FILE="+pf)
	tail := &tailBuffer{max: 2000}
	c.Stdout, c.Stderr = io.MultiWriter(r.log, tail), io.MultiWriter(r.log, tail)
	c.WaitDelay = 10 * time.Second
	start := time.Now()
	err := c.Run()
	proc.EndTree(c) // background processes the agent left behind (servers, watchers) end with it
	res := agentRun{duration: time.Since(start), tail: strings.TrimSpace(string(tail.b))}
	var ee *exec.ExitError
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.code, res.err = 124, fmt.Errorf("agent timed out after %s", r.timeout)
	case errors.Is(err, exec.ErrWaitDelay) && c.ProcessState != nil:
		res.code = c.ProcessState.ExitCode() // finished; a detached child held the output open
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

// reviewStep is the prompt line for the agent's own review loop, only where it applies: a
// task under [quality] review standard, or strict work (own clean pass before another tool's).
func reviewStep(p *project.Project, t *task.Task) string {
	prof := t.Profile // what close will use: pinned, unless the diff makes it strict
	if computed := profile.Compute(p, TaskChanges(p, t)); !t.ProfilePinned || computed == "strict" {
		prof = computed
	}
	if prof == "lite" && !t.ProfilePinned {
		prof = "standard" // the diff may still grow past lite
	}
	strictHint := ""
	if len(p.Config.Risk.SensitivePaths) > 0 && !t.ProfilePinned && prof != "strict" && p.Config.ReviewLoop() {
		strictHint = " If your change touches a sensitive path (aitk.toml [risk] sensitive_paths), the task becomes strict: your clean pass is still needed, and another tool's independent pass follows."
	}
	switch {
	case prof == "strict" && p.Config.ReviewLoop():
		return "\n   Then review your own change: record `aitk review pass " + t.ID + " --findings N`, fix what you found, run the check again, and repeat until a pass finds 0. Strict work also needs an independent pass by another tool after yours; your own passes never count as that one."
	case prof != "strict" && p.Config.ReviewRequired(prof):
		return "\n   Then review your own change: record `aitk review pass " + t.ID + " --findings N`, fix what you found, run the check again, and repeat until a pass finds 0." + strictHint
	}
	return ""
}
