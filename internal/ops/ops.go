// Package ops implements aitk operations. The CLI and the MCP server both call these,
// so their effects are identical (docs/spec/integrations.md).
package ops

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/v2/internal/agentsmd"
	"github.com/innaka-tech/ai-toolkit/v2/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/v2/internal/checkrun"
	"github.com/innaka-tech/ai-toolkit/v2/internal/claims"
	"github.com/innaka-tech/ai-toolkit/v2/internal/compat"
	"github.com/innaka-tech/ai-toolkit/v2/internal/conv"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/handoff"
	"github.com/innaka-tech/ai-toolkit/v2/internal/knowledge"
	"github.com/innaka-tech/ai-toolkit/v2/internal/plugins"
	"github.com/innaka-tech/ai-toolkit/v2/internal/profile"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/schema"
	"github.com/innaka-tech/ai-toolkit/v2/internal/secrets"
	"github.com/innaka-tech/ai-toolkit/v2/internal/session"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
	"github.com/innaka-tech/ai-toolkit/v2/internal/textx"
)

// Tool identifies the AI tool (or human) running aitk. AITK_TOOL overrides detection.
func Tool() string {
	if t := os.Getenv("AITK_TOOL"); t != "" {
		return project.Slug(t, 32)
	}
	// Tools inherit their parent's environment, so when one tool runs inside another
	// (e.g. Codex started from Claude Code) the innermost one is checked first.
	switch {
	case os.Getenv("CODEX_SESSION_ID") != "" || os.Getenv("CODEX_VERSION") != "" || os.Getenv("CODEX_SANDBOX") != "" || os.Getenv("CODEX_MANAGED_BY_NPM") != "":
		return "codex"
	case os.Getenv("GEMINI_CLI") != "":
		return "gemini-cli"
	case os.Getenv("OPENCODE") != "":
		return "opencode"
	case os.Getenv("CLAUDECODE") != "":
		return "claude-code"
	}
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return "human"
	}
	return "unknown"
}

// GuardText refuses text that would put a credential into the repository
// (agents paste tokens into summaries and notes surprisingly often).
func GuardText(what string, texts ...string) error {
	for _, text := range texts {
		for i, line := range strings.Split(text, "\n") {
			if fs := secrets.ScanLine(what, i+1, line); len(fs) > 0 {
				return apperr.New("E_SECRET", apperr.ExitGate, "remove the secret and refer to it by name (for example the environment variable that holds it)",
					"%s contains what looks like a credential (%s: %s); it was not recorded", what, fs[0].Rule, fs[0].Match)
			}
		}
	}
	return nil
}

// StopAfterFailures is the consecutive failing-check count at which agents must stop.
const StopAfterFailures = 2

// DefaultClaimTTL is how long a task claim lasts without renewal (task start renews it).
const DefaultClaimTTL = 4 * time.Hour

// Claim claims id for this worktree, failing when another worktree holds it.
func Claim(p *project.Project, id string, ttl time.Duration) (claims.Claim, error) {
	var c claims.Claim
	err := WithLock(p, func() error {
		t, err := task.Find(p, id)
		if err != nil {
			return apperr.TaskNotFound(id)
		}
		if h := claims.Holder(p, t.ID); h != nil {
			return apperr.New("E_TASK_CLAIMED", apperr.ExitConflict, "pick another task (aitk task list)", "task %s is claimed by %s until %s", t.ID, h.By, h.Expires)
		}
		c, err = claims.Take(p, t.ID, Tool(), ttl)
		return err
	})
	return c, err
}

// Release drops this worktree's claim (or any claim with force).
func Release(p *project.Project, id string, force bool) (bool, error) {
	var ok bool
	err := WithLock(p, func() error {
		var err error
		ok, err = claims.Release(p, id, force)
		return err
	})
	return ok, err
}

// WithLock runs fn while holding the project lock.
func WithLock(p *project.Project, fn func() error) error {
	l, err := fsx.Acquire(p.LockPath(), 10*time.Second)
	if errors.Is(err, fsx.ErrLocked) {
		return apperr.New("E_LOCKED", apperr.ExitConflict, "retry in a moment", "another aitk process holds the project lock")
	}
	if err != nil {
		return apperr.New("E_STATE_UNWRITABLE", apperr.ExitRuntime, "allow writes to the repository (for sandboxed agents: the working tree must be writable)",
			"cannot write aitk state in %s or %s: %v", p.StateDirs()[0], p.StateDirs()[1], err)
	}
	defer l.Release()
	return fn()
}

// ---------- init ----------

// InitResult reports what init created.
type InitResult struct {
	Created []string `json:"created"`
	Updated []string `json:"updated"`
	Kept    []string `json:"kept_hand_written,omitempty"` // existing docs aitk will not overwrite
	Check   string   `json:"check,omitempty"`
}

// Init bootstraps a v2 project in the repository containing dir. It never overwrites user content.
func Init(dir, name, check string, dryRun bool) (*InitResult, error) {
	p, err := project.Repo(dir)
	if err != nil {
		return nil, err
	}
	if project.IsV1(p.Root) {
		return nil, apperr.NeedsMigration()
	}
	if name == "" {
		name = project.Slug(filepath.Base(p.Root), 62)
	}
	if check == "" {
		check = project.DetectCheck(p.Root)
	}
	res := &InitResult{Check: check}
	write := func(rel string, content []byte) error {
		if fsx.Exists(p.Path(rel)) {
			return nil
		}
		res.Created = append(res.Created, rel)
		if dryRun {
			return nil
		}
		return fsx.WriteFile(p.Path(rel), content, 0o644)
	}
	cfg := fmt.Sprintf("# aitk project configuration — https://github.com/innaka-tech/ai-toolkit/blob/main/docs/spec/cli.md\n\n[project]\nname = %q\ndefault_branch = %q\n", project.Slug(name, 62), detectBranch(p))
	if check != "" {
		cfg += fmt.Sprintf("\n[check]\ncmd = %q\ntimeout = \"10m\"\n", check)
	} else {
		cfg += "\n# [check]\n# cmd = \"make test\"   # required for the Definition of Done to verify work\n"
	}
	cfg += "\n[risk]\n# Changes matching these globs make a task \"strict\" (risk analysis + independent review).\nsensitive_paths = []\n"
	state := map[string]any{"schema_version": 2, "project": map[string]any{"name": project.Slug(name, 62)}}
	stateJSON, _ := json.MarshalIndent(state, "", "  ")
	files := []struct {
		rel     string
		content string
	}{
		{project.ConfigFile, cfg},
		{project.StateFile, string(stateJSON) + "\n"},
		{project.ContextFile, "# Project context\n\n<!-- Purpose, stack, and constraints. Keep under 60 lines; aitk brief shows the first 3 lines. -->\n"},
		{project.DocsAI + "/goals.md", "# Goals\n\n<!-- Generated by aitk from ai-state.json goals. Manage with aitk goal add|status. -->\n"},
		{conv.File, conv.Template},
		{project.KnowDir + "/_inbox.md", "# Knowledge inbox\n\n<!-- New entries land here; `aitk knowledge compact` files them by topic. -->\n\n"},
		{project.TasksDir + "/.gitkeep", ""},
		{project.HandoffDir + "/.gitkeep", ""},
		{project.ADRDir + "/.gitkeep", ""},
	}
	for _, f := range files {
		if err := write(f.rel, []byte(f.content)); err != nil {
			return nil, err
		}
	}
	agents := p.Path(project.AgentsFile)
	b, _ := os.ReadFile(agents)
	if next := agentsmd.Apply(string(b)); next != string(b) {
		if len(b) == 0 {
			res.Created = append(res.Created, project.AgentsFile)
		} else {
			res.Updated = append(res.Updated, project.AgentsFile)
		}
		if !dryRun {
			if err := fsx.WriteFile(agents, []byte(next), 0o644); err != nil {
				return nil, err
			}
		}
	}
	if changed, err := compat.EnsureGitattributes(p, dryRun); err != nil {
		return nil, err
	} else if changed {
		res.Updated = append(res.Updated, ".gitattributes")
	}
	if !dryRun {
		if q, err := project.Open(p.Root); err == nil {
			handoff.WriteIndex(q)
			res.Kept = compat.HandWritten(q)
		}
	}
	return res, nil
}

func detectBranch(p *project.Project) string {
	for _, b := range []string{"main", "master", "trunk"} {
		// rev-parse sees loose, packed, and reftable refs alike.
		if _, err := gitx.Run(p.Root, "rev-parse", "--verify", "-q", "refs/heads/"+b); err == nil {
			return b
		}
	}
	if cur := branchOf(p); cur != "" && cur != "HEAD" && !strings.Contains(cur, "/") {
		return cur // a repository whose only branch has another name
	}
	return "main"
}

// ---------- tasks ----------

// NewTaskInput holds task creation fields.
type NewTaskInput struct {
	Title, ID, Goal, Profile, Objective string
	Criteria, Tags                      []string
}

// TaskNew creates a task in status todo.
func TaskNew(p *project.Project, in NewTaskInput) (*task.Task, error) {
	in.Title = strings.TrimSpace(in.Title)
	if len([]rune(in.Title)) < 3 {
		return nil, apperr.Usage("task title must be at least 3 characters")
	}
	if err := GuardText("task", append([]string{in.Title, in.Objective}, in.Criteria...)...); err != nil {
		return nil, err
	}
	if in.Goal != "" && FindGoal(p, in.Goal) == nil {
		return nil, apperr.New("E_USAGE", apperr.ExitUsage, `aitk goal add "<title>"`, "goal %s does not exist", in.Goal)
	}
	var t *task.Task
	err := WithLock(p, func() error {
		id := in.ID
		if id == "" {
			id = task.NewID(p)
		} else if !task.IDPattern.MatchString(id) {
			return apperr.Usage("invalid task id %q (letters, digits, '-', starting with a letter)", id)
		} else if _, err := task.Find(p, id); err == nil {
			return apperr.New("E_USAGE", apperr.ExitUsage, "aitk task list", "task %s already exists", id)
		}
		now := task.Now()
		prof, pinned := in.Profile, in.Profile != ""
		if prof == "" {
			prof = "standard"
		}
		t = &task.Task{Meta: task.Meta{ID: id, Title: textx.Truncate(in.Title, 120), Status: task.Todo, Profile: prof, ProfilePinned: pinned,
			Goal: in.Goal, Tags: slugs(in.Tags), Created: now, Updated: now, CreatedBy: Tool()},
			Body: task.NewBody(in.Objective, in.Criteria)}
		return task.Save(p, t)
	})
	return t, err
}

func slugs(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		s = project.Slug(s, 62)
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// TaskStart makes id the active task of this worktree and marks it in_progress.
func TaskStart(p *project.Project, id string) (*task.Task, error) {
	var t *task.Task
	err := WithLock(p, func() error {
		var err error
		if t, err = task.Find(p, id); err != nil {
			return apperr.TaskNotFound(id)
		}
		if t.Status == task.Done || t.Status == task.Cancelled {
			return apperr.New("E_USAGE", apperr.ExitUsage, fmt.Sprintf("aitk task update %s --status todo", t.ID), "task %s is %s; reopen it first", t.ID, t.Status)
		}
		if h := claims.Holder(p, t.ID); h != nil {
			return apperr.New("E_TASK_CLAIMED", apperr.ExitConflict, "pick another task (aitk task list), or wait until "+h.Expires,
				"task %s is claimed by %s in another worktree until %s", t.ID, h.By, h.Expires)
		}
		if _, err := claims.Take(p, t.ID, Tool(), DefaultClaimTTL); err != nil {
			return err
		}
		now := task.Now()
		if t.Status == task.Todo || t.Status == task.Blocked {
			t.Status = task.InProgress
		}
		if t.Base == "" { // where the task's work begins: its risk profile includes everything since
			t.Base = gitx.Head(p.Root)
		}
		t.Updated = now
		t.AddWorker(Tool())
		if b := branchOf(p); b != "" {
			t.Branch = b
		}
		if err := task.Save(p, t); err != nil {
			return err
		}
		s := session.Load(p)
		s.SetActive(t.ID, now, Tool())
		return session.Save(p, s)
	})
	return t, err
}

func branchOf(p *project.Project) string {
	b, _ := os.ReadFile(filepath.Join(p.GitDir, "HEAD"))
	s := strings.TrimSpace(string(b))
	return strings.TrimPrefix(s, "ref: refs/heads/")
}

// UpdateInput holds task update fields; zero values mean "no change".
type UpdateInput struct {
	Status  string
	ACDone  []int // 1-based
	AddAC   []string
	Note    string
	Tags    []string
	Profile string
	Reason  string // for blocked
}

// TaskUpdate edits structured fields without hand-editing YAML.
func TaskUpdate(p *project.Project, id string, in UpdateInput) (*task.Task, error) {
	if err := GuardText("task update", append([]string{in.Note, in.Reason}, in.AddAC...)...); err != nil {
		return nil, err
	}
	var t *task.Task
	err := WithLock(p, func() error {
		var err error
		if id == "" {
			id = session.Load(p).Active()
			if id == "" {
				return apperr.NoActiveTask()
			}
		}
		if t, err = task.Find(p, id); err != nil {
			return apperr.TaskNotFound(id)
		}
		if in.Status != "" {
			switch in.Status {
			case task.Todo, task.InProgress, task.Blocked, task.Cancelled:
				t.Status = in.Status
				if in.Status == task.Cancelled || in.Status == task.Blocked {
					if s := session.Load(p); strings.EqualFold(s.Active(), t.ID) {
						s.LastTask, s.LastTaskAt = t.ID, task.Now()
						s.SetActive("", "", "")
						session.Save(p, s)
					}
					claims.Release(p, t.ID, false)
				}
			case task.Done, task.Implemented, task.InReview:
				return apperr.New("E_USAGE", apperr.ExitUsage, `aitk close --summary "…" --knowledge "…"`,
					"status %s is set by close after the Definition of Done, not by update", in.Status)
			default:
				return apperr.Usage("invalid status %q (todo, in_progress, blocked, or cancelled)", in.Status)
			}
		}
		if in.Profile != "" {
			if !contains([]string{"lite", "standard", "strict"}, in.Profile) {
				return apperr.Usage("invalid profile %q", in.Profile)
			}
			t.Profile, t.ProfilePinned = in.Profile, true
		}
		t.AddCriteria(in.AddAC)
		if err := t.MarkCriteria(in.ACDone); err != nil {
			return apperr.Usage("%v", err)
		}
		if in.Note != "" || in.Reason != "" {
			note := in.Note
			if in.Reason != "" {
				note = "Blocked: " + in.Reason
			}
			sec := ""
			if s, ok := sectionOf(t, "Notes"); ok {
				sec = s + "\n"
			}
			t.Body = setSection(t, "Notes", sec+"- "+task.Now()+" "+note)
		}
		if len(in.Tags) > 0 {
			t.Tags = slugs(append(t.Tags, in.Tags...))
		}
		t.Updated = task.Now()
		return task.Save(p, t)
	})
	return t, err
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// ---------- check ----------

// CheckResult is the outcome of aitk check.
type CheckResult struct {
	task.Check
	Task     string `json:"task,omitempty"`
	TimedOut bool   `json:"timed_out,omitempty"`
	Failures int    `json:"consecutive_failures,omitempty"`
}

// Check runs check.cmd and records evidence on the active task (if any).
func Check(p *project.Project, stream bool) (*CheckResult, error) {
	cmd := p.Config.Check.Cmd
	if cmd == "" {
		return nil, apperr.New("E_CHECK_NOT_CONFIGURED", apperr.ExitGate, `add [check] cmd = "<command>" to aitk.toml`, "no check command configured")
	}
	timeout, err := time.ParseDuration(p.Config.CheckTimeout())
	if err != nil {
		return nil, apperr.Usage("invalid check.timeout %q", p.Config.CheckTimeout())
	}
	tree := profile.Fingerprint(p)
	r := checkrun.Run(p.Root, cmd, timeout, stream)
	res := &CheckResult{Check: task.Check{Cmd: cmd, ExitCode: r.ExitCode, DurationMs: r.DurationMs,
		Summary: checkrun.Summary(strings.TrimSpace(r.Output), 2000), At: task.Now(), Tree: tree}, TimedOut: r.TimedOut}
	err = WithLock(p, func() error {
		s := session.Load(p)
		c := res.Check
		s.LastCheck = &c
		return session.Save(p, s)
	})
	if err != nil {
		return res, err
	}
	if id := session.Load(p).Active(); id != "" {
		err := WithLock(p, func() error {
			t, err := task.Find(p, id)
			if err != nil {
				return nil
			}
			if t.Evidence == nil {
				t.Evidence = &task.Evidence{}
			}
			c := res.Check
			t.Evidence.Check = &c
			if c.ExitCode != 0 {
				t.Evidence.CheckFailures++
			} else {
				t.Evidence.CheckFailures = 0
			}
			res.Failures = t.Evidence.CheckFailures
			t.Updated = task.Now() // check does not make a tool a worker: reviewers run it too
			res.Task = t.ID
			return task.Save(p, t)
		})
		if err != nil {
			return res, err
		}
	}
	if r.ExitCode != 0 {
		msg := "check failed"
		if r.TimedOut {
			msg = "check timed out after " + p.Config.CheckTimeout()
		}
		if res.Failures >= StopAfterFailures {
			return res, apperr.New("E_CHECK_FAILED", apperr.ExitRuntime,
				fmt.Sprintf(`stop and ask the user, or hand over: aitk close --status blocked --summary "<what you tried>" --knowledge "<what you learned>"`),
				"%s (exit %d); this is failure %d in a row for %s, so stop retrying", msg, r.ExitCode, res.Failures, res.Task)
		}
		return res, apperr.New("E_CHECK_FAILED", apperr.ExitRuntime, "fix the failures, then run: aitk check", "%s (exit %d)", msg, r.ExitCode)
	}
	return res, nil
}

// ---------- close ----------

// CloseInput holds close fields.
type CloseInput struct {
	Summary   string
	Knowledge string // "none" allowed
	Next      []string
	Status    string // optional: in_progress or blocked to close a session without finishing
	Tags      []string
}

// CloseResult reports what close did.
type CloseResult struct {
	Task      string   `json:"task"`
	Status    string   `json:"status"`
	Profile   string   `json:"profile"`
	Handoff   string   `json:"handoff"`
	Knowledge bool     `json:"knowledge_recorded"`
	Compacted bool     `json:"compacted,omitempty"`
	Notes     []string `json:"notes,omitempty"`
}

// Close applies the Definition of Done (docs/spec/workflow.md §5) and records evidence.
func Close(p *project.Project, in CloseInput) (*CloseResult, error) {
	in.Summary = strings.TrimSpace(in.Summary)
	in.Knowledge = strings.TrimSpace(in.Knowledge)
	if in.Summary == "" {
		return nil, apperr.New("E_DOD_SUMMARY", apperr.ExitGate, `aitk close --summary "<what changed>" --knowledge "<finding|none>"`, "--summary is required")
	}
	if in.Knowledge == "" {
		return nil, apperr.New("E_DOD_KNOWLEDGE", apperr.ExitGate, `add --knowledge "<lasting finding>" (or --knowledge none)`, "--knowledge is required (use \"none\" when there is no lasting finding)")
	}
	if err := GuardText("close", append([]string{in.Summary, in.Knowledge}, in.Next...)...); err != nil {
		return nil, err
	}
	if in.Status != "" && in.Status != task.InProgress && in.Status != task.Blocked {
		return nil, apperr.Usage("--status only accepts in_progress or blocked (to hand over unfinished work)")
	}
	var res *CloseResult
	defer func() {
		if res == nil || res.Handoff == "" || len(p.Config.Plugins.Enabled) == 0 {
			return
		}
		for _, r := range plugins.Call(p, "close.after", map[string]any{"task": res.Task, "status": res.Status, "handoff_path": res.Handoff, "knowledge": in.Knowledge}, map[string]any{"id": res.Task}) {
			if r.Err != "" {
				res.Notes = append(res.Notes, "plugin "+r.Plugin+": "+r.Err)
			}
		}
	}()
	err := WithLock(p, func() error {
		s := session.Load(p)
		var t *task.Task
		if id := s.Active(); id != "" {
			var err error
			if t, err = task.Find(p, id); err != nil {
				return apperr.New("E_TASK_NOT_FOUND", apperr.ExitUsage, "aitk doctor --fix (clears it), then aitk task next",
					"the active task %s does not exist on this branch (switched branches?)", id)
			}
			if t.Status == task.Cancelled || t.Status == task.Done {
				return apperr.New("E_USAGE", apperr.ExitUsage, fmt.Sprintf("aitk task update %s --status todo (to reopen it), or aitk task next", t.ID),
					"task %s is %s; it cannot be closed again", t.ID, t.Status)
			}
		}
		changes := TaskChanges(p, t)
		computed := profile.Compute(p, changes)
		if t == nil {
			if computed != "lite" || in.Status != "" {
				return apperr.NoActiveTask()
			}
			now := task.Now()
			t = &task.Task{Meta: task.Meta{ID: task.NewID(p), Title: implicitTitle(in.Summary), Status: task.InProgress, Profile: "lite",
				Created: now, Updated: now, CreatedBy: Tool()}, Body: task.NewBody(in.Summary, nil)}
			if s.LastCheck != nil {
				c := *s.LastCheck
				t.Evidence = &task.Evidence{Check: &c}
			}
		}
		// The diff decides; a pinned profile applies unless the diff touches sensitive paths,
		// which always makes the task strict.
		prof := computed
		if t.ProfilePinned && computed != "strict" {
			prof = t.Profile
		}
		t.AddWorker(Tool())
		res = &CloseResult{Task: t.ID, Profile: prof}
		// Validate every input before writing anything, so a rejected close changes nothing.
		var entry *knowledge.Entry
		if !strings.EqualFold(in.Knowledge, "none") {
			tags := slugs(append(append([]string{}, in.Tags...), t.Tags...))
			topic := "general"
			if len(tags) > 0 {
				topic = tags[0]
			}
			entry = &knowledge.Entry{Text: in.Knowledge, Date: task.Now()[:10], Topic: topic, Task: t.ID, Tags: tags, Source: "close"}
			if err := schema.Validate("knowledge-entry", entry); err != nil {
				return apperr.New("E_SCHEMA", apperr.ExitGate, "shorten --knowledge to one or two sentences (max 4000 characters)", "knowledge entry invalid: %v", err)
			}
		}
		target, err := dod(p, t, prof, in)
		if err != nil {
			return err
		}
		now := task.Now()
		t.Status, t.Profile, t.Updated = target, prof, now
		if target == task.Done {
			t.Closed = now
		}
		if t.Evidence == nil {
			t.Evidence = &task.Evidence{}
		}
		if in.Status == "" && IsBugTask(t) && len(t.Evidence.RegressionTests) == 0 {
			t.Evidence.RegressionTests = capList(regressionTests(p, t), 10)
		}
		if cs := t.Criteria(); len(cs) > 0 {
			n := 0
			for _, c := range cs {
				if c.Done {
					n++
				}
			}
			t.Evidence.Acceptance = &task.Acceptance{Total: len(cs), Done: n}
		}
		t.Evidence.Commits = mergeUnique(t.Evidence.Commits, profile.Commits(p, t.Created, t.ID))
		for _, c := range changes {
			t.Paths = mergeUnique(t.Paths, []string{c.Path})
		}
		t.Body = setSection(t, "Evidence", evidenceText(t))
		if err := task.Save(p, t); err != nil {
			return err
		}
		outcome := map[string]string{task.Done: "done", task.InReview: "progress", task.Implemented: "progress", task.InProgress: "progress", task.Blocked: "blocked"}[target]
		files := make([]string, 0, len(changes))
		for _, c := range changes {
			files = append(files, c.Path)
		}
		h, err := handoff.Write(p, handoff.Meta{Task: t.ID, At: now, By: Tool(), Outcome: outcome, TaskStatus: target,
			Check: t.Evidence.Check, Commits: t.Evidence.Commits, Files: files, Next: in.Next}, "## "+t.Title+"\n\n"+in.Summary+"\n")
		if err != nil {
			return err
		}
		res.Handoff, res.Status = h.File, target
		if entry != nil {
			if err := knowledge.Add(p, *entry); err != nil {
				return err
			}
			res.Knowledge = true
			if knowledge.InboxCount(p) > p.Config.InboxLimit() {
				if _, err := knowledge.Compact(p, p.Config.ArchiveAfterDays(), false); err == nil {
					res.Compacted = true
				}
			}
		}
		if target == task.Done || target == task.Blocked {
			s.LastTask, s.LastTaskAt = t.ID, now
			s.SetActive("", "", "")
			claims.Release(p, t.ID, false)
		}
		total := 0
		for _, c := range changes {
			total += c.Lines
		}
		if len(changes) > 40 || total > 2000 {
			res.Notes = append(res.Notes, fmt.Sprintf("large change (%d files, %d lines): smaller tasks are easier to review and safer to revert", len(changes), total))
		}
		// Last, after aitk's own records: the plan file of an imported task.
		if target == task.Done {
			if ok, err := syncSource(p, t); ok {
				res.Notes = append(res.Notes, "checked off in "+fmt.Sprint(t.Legacy["file"]))
			} else if err != nil {
				res.Notes = append(res.Notes, "could not check off the source item: "+err.Error())
			}
		}
		if target == task.InReview {
			if prof == "strict" && !reviewDone(t) {
				res.Notes = append(res.Notes, "strict task needs ≥2 review passes (last with 0 findings, one by a tool that did not work on it) before it can be done: aitk review pass --findings N")
			} else {
				res.Notes = append(res.Notes, "waiting for user acceptance: a person runs aitk uat accept "+t.ID+" (script: aitk uat script "+t.ID+")")
			}
		}
		return session.Save(p, s)
	})
	return res, err
}

func rank(p string) int { return map[string]int{"lite": 0, "standard": 1, "strict": 2}[p] }

func implicitTitle(summary string) string {
	t := textx.Truncate(textx.FirstLine(summary), 120)
	if len([]rune(t)) < 3 {
		t = "Quick change"
	}
	return t
}

// dod returns the status the task may move to, or a gate error (docs/spec/workflow.md §5):
// check → criteria → risk analysis (strict) → ASVS checklist (strict) → security audit →
// independent review (strict) → user acceptance.
func dod(p *project.Project, t *task.Task, prof string, in CloseInput) (string, error) {
	if in.Status != "" { // handing over unfinished work: no DoD
		return in.Status, nil
	}
	tree := profile.Fingerprint(p)
	if p.Config.Check.Cmd != "" {
		c := (*task.Check)(nil)
		if t.Evidence != nil {
			c = t.Evidence.Check
		}
		switch {
		case c == nil:
			return "", apperr.New("E_DOD_CHECK_STALE", apperr.ExitGate, "aitk check", "no check has been recorded for this task")
		case c.ExitCode != 0:
			return "", apperr.New("E_DOD_CHECK_STALE", apperr.ExitGate, "fix the failures, then run: aitk check", "the last check failed (exit %d)", c.ExitCode)
		case c.Tree != tree && !profile.Fresh(p, c.Tree):
			return "", apperr.New("E_DOD_CHECK_STALE", apperr.ExitGate, "aitk check", "files changed after the last passing check")
		}
	}
	cs := t.Criteria()
	if len(cs) == 0 && prof != "lite" {
		return "", apperr.New("E_DOD_ACCEPTANCE", apperr.ExitGate, fmt.Sprintf(`aitk task update %s --add-ac "Given …, when …, then …"`, t.ID), "a %s task needs at least one acceptance criterion", prof)
	}
	var open []string
	for i, c := range cs {
		if !c.Done {
			open = append(open, fmt.Sprintf("%d", i+1))
		}
	}
	if len(open) > 0 {
		return "", apperr.New("E_DOD_ACCEPTANCE", apperr.ExitGate, fmt.Sprintf("aitk task update %s --ac-done %s", t.ID, strings.Join(open, ",")), "acceptance criteria not yet satisfied: %s", strings.Join(open, ", "))
	}
	if p.Config.RegressionTestsRequired() && IsBugTask(t) && !regressionTested(p, t) {
		return "", apperr.New("E_DOD_REGRESSION_TEST", apperr.ExitGate,
			"add a test that fails without the fix and passes with it, then run: aitk check (or remove the bug tag if this is not a bug fix)",
			"bug fix %s changes no test file: add a regression test so the bug cannot come back unnoticed", t.ID)
	}
	if prof == "strict" {
		if !t.RiskFilled() {
			return "", apperr.New("E_DOD_RISK", apperr.ExitGate, "fill the Risk section of "+t.File+" (Impact, Security, Rollback, Validation)", "strict task without risk analysis")
		}
		if p.Config.ASVSLevel() > 0 {
			present, openSec := securityOpen(t)
			if !present {
				return "", apperr.New("E_DOD_SECURITY", apperr.ExitGate, "aitk security checklist "+t.ID, "strict task without the OWASP ASVS security checklist")
			}
			if len(openSec) > 0 {
				return "", apperr.New("E_DOD_SECURITY", apperr.ExitGate, "verify each item in "+t.File+" (check it, or write \"N/A: <reason>\" and check it)",
					"security checklist items not verified: %s", strings.Join(openSec, ", "))
			}
		}
	}
	if p.Config.AuditRequired(prof) {
		a := (*task.Audit)(nil)
		if t.Evidence != nil {
			a = t.Evidence.Audit
		}
		switch {
		case a == nil:
			return "", apperr.New("E_DOD_AUDIT", apperr.ExitGate, "aitk audit", "a %s task needs a security audit (dependencies, static analysis, secrets)", prof)
		case !a.Passed:
			return "", apperr.New("E_DOD_AUDIT", apperr.ExitGate, "fix what the scanners report, then run: aitk audit", "the last security audit found problems")
		case a.Tree != tree && !profile.Fresh(p, a.Tree):
			return "", apperr.New("E_DOD_AUDIT", apperr.ExitGate, "aitk audit", "files changed after the last passing audit")
		}
	}
	if prof == "strict" && !reviewDone(t) {
		return task.InReview, nil
	}
	if p.Config.UATRequired(prof) && (t.Evidence == nil || t.Evidence.UAT == nil || t.Evidence.UAT.Status != "accepted") {
		return task.InReview, nil
	}
	return task.Done, nil
}

// reviewDone: ≥2 passes, the last with 0 findings, and at least one pass by a tool that did
// not work on the task (workers, the creator, and the tool closing it).
func reviewDone(t *task.Task) bool {
	if t.Review == nil || len(t.Review.Passes) < 2 || t.Review.Passes[len(t.Review.Passes)-1].Findings != 0 {
		return false
	}
	implementers := map[string]bool{Tool(): true}
	if t.CreatedBy != "" {
		implementers[t.CreatedBy] = true
	}
	for _, w := range t.Workers {
		implementers[w] = true
	}
	for _, pass := range t.Review.Passes {
		if pass.By != "" && pass.By != "unknown" && !implementers[pass.By] {
			return true
		}
	}
	return false
}

// ReviewPass records a bug-hunt pass on the active task.
func ReviewPass(p *project.Project, findings int) (*task.Task, error) {
	var t *task.Task
	err := WithLock(p, func() error {
		id := session.Load(p).Active()
		if id == "" {
			return apperr.NoActiveTask()
		}
		var err error
		if t, err = task.Find(p, id); err != nil {
			return apperr.TaskNotFound(id)
		}
		if t.Review == nil {
			t.Review = &task.Review{}
		}
		t.Review.Passes = append(t.Review.Passes, task.Pass{N: len(t.Review.Passes) + 1, By: Tool(), At: task.Now(), Findings: findings})
		if t.Status == task.Implemented || t.Status == task.InProgress {
			t.Status = task.InReview
		}
		t.Updated = task.Now()
		return task.Save(p, t)
	})
	return t, err
}

func evidenceText(t *task.Task) string {
	var b strings.Builder
	b.WriteString("<!-- Generated by aitk close. -->\n")
	if e := t.Evidence; e != nil {
		if c := e.Check; c != nil {
			fmt.Fprintf(&b, "- Check: `%s` → exit %d at %s (%d ms)\n", c.Cmd, c.ExitCode, c.At, c.DurationMs)
		}
		if a := e.Acceptance; a != nil {
			fmt.Fprintf(&b, "- Acceptance criteria: %d/%d satisfied\n", a.Done, a.Total)
		}
		if len(e.Commits) > 0 {
			fmt.Fprintf(&b, "- Commits: %s\n", strings.Join(e.Commits, ", "))
		}
	}
	if t.Review != nil {
		for _, ps := range t.Review.Passes {
			fmt.Fprintf(&b, "- Review pass %d by %s: %d findings\n", ps.N, ps.By, ps.Findings)
		}
	}
	return b.String()
}

func mergeUnique(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// IsBugTask reports whether t is a bug fix (tagged bug).
func IsBugTask(t *task.Task) bool {
	return contains(t.Tags, "bug")
}

// regressionTested reports whether a bug task changed a test file. Evidence recorded by a
// successful close counts (a later UAT acceptance, after the work is committed or merged, still
// sees it); otherwise the task's own changes are inspected.
func regressionTested(p *project.Project, t *task.Task) bool {
	if t.Evidence != nil && len(t.Evidence.RegressionTests) > 0 {
		return true
	}
	return len(regressionTests(p, t)) > 0
}

// regressionTests lists test files added or modified by the task: in commits made since it was
// created that do not name another task in an AI-Task trailer, and in uncommitted changes.
// Deleting a test is not a regression test.
func regressionTests(p *project.Project, t *task.Task) []string {
	var tests []string
	seen := map[string]bool{}
	add := func(path string) {
		if IsTestPath(path) && !seen[path] {
			seen[path] = true
			tests = append(tests, path)
		}
	}
	for _, c := range taskCommits(p, t) {
		out, _ := gitx.RunRaw(p.Root, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", "--root", "--diff-filter=AMR", c)
		for _, f := range strings.Split(string(out), "\x00") {
			if f != "" {
				add(f)
			}
		}
	}
	out, _ := gitx.RunRaw(p.Root, "status", "--porcelain", "-z", "--untracked-files=all")
	toks := strings.Split(string(out), "\x00")
	for i := 0; i < len(toks); i++ {
		e := toks[i]
		if len(e) < 4 {
			continue
		}
		if e[0] != 'D' && e[1] != 'D' {
			add(e[3:])
		}
		if e[0] == 'R' || e[0] == 'C' {
			i++
		}
	}
	sort.Strings(tests)
	return tests
}

// taskCommits are the task's own commits (see profile.CommitsFull).
func taskCommits(p *project.Project, t *task.Task) []string {
	return profile.CommitsFull(p, t.Created, t.ID)
}

// parentOf is the commit before c, or the empty tree when c is the first commit.
func parentOf(p *project.Project, c string) string {
	if parent, err := gitx.Run(p.Root, "rev-parse", "--verify", "-q", c+"^"); err == nil && parent != "" {
		return parent
	}
	return profile.EmptyTree(p)
}

// TaskChanges are the changes that make up t's risk profile: the branch's changes since the
// default branch plus everything since the task started (on the default branch, the first
// part is only uncommitted work). Without a task, the branch's changes.
func TaskChanges(p *project.Project, t *task.Task) []profile.Change {
	if t == nil {
		// A close without a task covers the work since the last close (its handoff).
		if hs := handoff.List(p); len(hs) > 0 {
			// Commits from the second of the last close on count too: when unsure, include.
			if at, err := time.Parse("2006-01-02T15:04:05Z", hs[0].At); err == nil {
				since := at.Add(-time.Second).UTC().Format("2006-01-02T15:04:05Z")
				if cs := profile.CommitsFull(p, since, ""); len(cs) > 0 {
					return profile.DiffSince(p, parentOf(p, cs[len(cs)-1]))
				}
			}
		}
		// No close yet: everything since aitk was set up (the commit that added aitk.toml).
		if added, err := gitx.Run(p.Root, "log", "--diff-filter=A", "--format=%H", "--", project.ConfigFile); err == nil && added != "" {
			lines := strings.Fields(added)
			return profile.DiffSince(p, parentOf(p, lines[len(lines)-1]))
		}
		return profile.Diff(p)
	}
	base := t.Base
	if base == "" { // started by an older aitk: the parent of its first commit
		if cs := taskCommits(p, t); len(cs) > 0 {
			base = parentOf(p, cs[len(cs)-1])
		}
	}
	return profile.DiffSince(p, base)
}

func containsFold(xs []string, s string) bool {
	for _, x := range xs {
		if strings.EqualFold(strings.TrimSpace(x), s) {
			return true
		}
	}
	return false
}

var testFile = regexp.MustCompile(`((?i:_test\.[a-z0-9]+|\.(test|spec)\.(js|jsx|ts|tsx|mjs|cjs|vue|svelte|py|rb|php|go|java|kt|cs|swift|dart|ex|exs|rs|c|cc|cpp|scala|sh|lua)|_spec\.rb|_tests?\.exs?)|^tests?\.py|^test_[^/]+\.py|^conftest\.py|[A-Za-z0-9](Test|Tests|Spec|IT)\.(java|kt|cs|php|swift|scala|groovy))$`)

// IsTestPath reports whether a repository path is a test file, by common naming conventions.
func IsTestPath(path string) bool {
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, "/")
	for _, d := range parts[:len(parts)-1] {
		switch strings.ToLower(d) {
		case "test", "tests", "__tests__", "spec", "e2e", "testing", "testdata", "integration-tests":
			return true
		}
	}
	return testFile.MatchString(parts[len(parts)-1])
}

func capList(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}
