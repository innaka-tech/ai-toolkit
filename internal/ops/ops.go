// Package ops implements aitk operations. The CLI and the MCP server both call these,
// so their effects are identical (docs/spec/integrations.md).
package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/internal/agentsmd"
	"github.com/innaka-tech/ai-toolkit/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/internal/checkrun"
	"github.com/innaka-tech/ai-toolkit/internal/claims"
	"github.com/innaka-tech/ai-toolkit/internal/compat"
	"github.com/innaka-tech/ai-toolkit/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/internal/handoff"
	"github.com/innaka-tech/ai-toolkit/internal/knowledge"
	"github.com/innaka-tech/ai-toolkit/internal/plugins"
	"github.com/innaka-tech/ai-toolkit/internal/profile"
	"github.com/innaka-tech/ai-toolkit/internal/project"
	"github.com/innaka-tech/ai-toolkit/internal/schema"
	"github.com/innaka-tech/ai-toolkit/internal/session"
	"github.com/innaka-tech/ai-toolkit/internal/task"
	"github.com/innaka-tech/ai-toolkit/internal/textx"
)

// Tool identifies the AI tool (or human) running aitk. AITK_TOOL overrides detection.
func Tool() string {
	if t := os.Getenv("AITK_TOOL"); t != "" {
		return project.Slug(t, 32)
	}
	switch {
	case os.Getenv("CLAUDECODE") != "":
		return "claude-code"
	case os.Getenv("GEMINI_CLI") != "":
		return "gemini-cli"
	case os.Getenv("OPENCODE") != "":
		return "opencode"
	case os.Getenv("CODEX_SANDBOX") != "" || os.Getenv("CODEX_MANAGED_BY_NPM") != "":
		return "codex"
	}
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return "human"
	}
	return "unknown"
}

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
	if err != nil {
		return apperr.New("E_LOCKED", apperr.ExitConflict, "retry in a moment", "another aitk process holds the project lock")
	}
	defer l.Release()
	return fn()
}

// ---------- init ----------

// InitResult reports what init created.
type InitResult struct {
	Created []string `json:"created"`
	Updated []string `json:"updated"`
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
		{project.DocsAI + "/goals.md", "# Goals\n"},
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
		}
	}
	return res, nil
}

func detectBranch(p *project.Project) string {
	for _, b := range []string{"main", "master", "trunk"} {
		if _, err := os.Stat(filepath.Join(p.CommonDir, "refs", "heads", b)); err == nil {
			return b
		}
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
		t.Updated = now
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
			if !contains(task.Statuses, in.Status) {
				return apperr.Usage("invalid status %q (one of %s)", in.Status, strings.Join(task.Statuses, ", "))
			}
			t.Status = in.Status
		}
		if in.Profile != "" {
			if !contains([]string{"lite", "standard", "strict"}, in.Profile) {
				return apperr.Usage("invalid profile %q", in.Profile)
			}
			t.Profile, t.ProfilePinned = in.Profile, true
		}
		cs := t.Criteria()
		for _, c := range in.AddAC {
			cs = append(cs, task.Criterion{Text: c})
		}
		for _, n := range in.ACDone {
			if n < 1 || n > len(cs) {
				return apperr.Usage("criterion %d does not exist (task has %d)", n, len(cs))
			}
			cs[n-1].Done = true
		}
		if len(in.AddAC) > 0 || len(in.ACDone) > 0 {
			t.SetCriteria(cs)
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
			t.Updated = task.Now()
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
		changes := profile.Diff(p)
		computed := profile.Compute(p, changes)
		var t *task.Task
		if id := s.Active(); id != "" {
			var err error
			if t, err = task.Find(p, id); err != nil {
				return apperr.TaskNotFound(id)
			}
		}
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
		prof := computed
		if t.ProfilePinned || rank(t.Profile) > rank(computed) {
			prof = t.Profile
		}
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
		if cs := t.Criteria(); len(cs) > 0 {
			n := 0
			for _, c := range cs {
				if c.Done {
					n++
				}
			}
			t.Evidence.Acceptance = &task.Acceptance{Total: len(cs), Done: n}
		}
		t.Evidence.Commits = mergeUnique(t.Evidence.Commits, profile.Commits(p, t.Created))
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
			s.SetActive("", "", "")
			claims.Release(p, t.ID, false)
		}
		if target == task.InReview {
			res.Notes = append(res.Notes, "strict task needs ≥2 review passes (last with 0 findings, one by a different tool/session) before it can be done: aitk review pass --findings N")
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

// dod returns the status the task may move to, or a gate error.
func dod(p *project.Project, t *task.Task, prof string, in CloseInput) (string, error) {
	if in.Status != "" { // handing over unfinished work: no DoD
		return in.Status, nil
	}
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
		case c.Tree != profile.Fingerprint(p):
			return "", apperr.New("E_DOD_CHECK_STALE", apperr.ExitGate, "aitk check", "files changed after the last passing check")
		}
	}
	if prof == "lite" {
		return task.Done, nil
	}
	cs := t.Criteria()
	if len(cs) == 0 {
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
	if prof == "standard" {
		return task.Done, nil
	}
	if !t.RiskFilled() {
		return "", apperr.New("E_DOD_RISK", apperr.ExitGate, "fill the Risk section of "+t.File+" (Impact, Security, Rollback, Validation)", "strict task without risk analysis")
	}
	if reviewDone(t) {
		return task.Done, nil
	}
	return task.InReview, nil
}

// reviewDone: ≥2 passes, the last with 0 findings, at least one by a tool other than the implementer.
func reviewDone(t *task.Task) bool {
	if t.Review == nil || len(t.Review.Passes) < 2 || t.Review.Passes[len(t.Review.Passes)-1].Findings != 0 {
		return false
	}
	for _, pass := range t.Review.Passes {
		if pass.By != t.CreatedBy {
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
