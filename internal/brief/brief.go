// Package brief renders the bounded session brief (docs/spec/workflow.md §7).
package brief

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/conv"
	"github.com/innaka-tech/ai-toolkit/v2/internal/handoff"
	"github.com/innaka-tech/ai-toolkit/v2/internal/knowledge"
	"github.com/innaka-tech/ai-toolkit/v2/internal/plugins"
	"github.com/innaka-tech/ai-toolkit/v2/internal/profile"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/session"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
	"github.com/innaka-tech/ai-toolkit/v2/internal/textx"
)

// Brief is the structured form (--json).
type Brief struct {
	Project             ProjectInfo        `json:"project"`
	Task                *TaskInfo          `json:"task,omitempty"`
	Open                []TaskRef          `json:"open_tasks,omitempty"`
	Legacy              string             `json:"legacy_current_task,omitempty"`
	Handoff             *HandoffInfo       `json:"last_handoff,omitempty"`
	Knowledge           []knowledge.Scored `json:"knowledge,omitempty"`
	Rules               []string           `json:"rules"`
	Next                []string           `json:"next_commands"`
	Profile             string             `json:"profile"`
	Tokens              int                `json:"tokens"`
	Budget              int                `json:"budget"`
	Dropped             int                `json:"knowledge_dropped,omitempty"`
	Changed             []profile.Change   `json:"changed_files,omitempty"`
	Conventions         []string           `json:"conventions,omitempty"`
	ConventionsUnfilled bool               `json:"conventions_unfilled,omitempty"`
	Goal                *GoalInfo          `json:"goal,omitempty"`
	Plugins             []PluginSection    `json:"plugin_sections,omitempty"`
	PluginErr           []string           `json:"plugin_errors,omitempty"`
	Markdown            string             `json:"markdown"`
}

// PluginSection is contributed by a plugin's brief.sections hook.
type PluginSection struct {
	Plugin string  `json:"plugin"`
	Title  string  `json:"title"`
	Body   string  `json:"body"`
	Rank   float64 `json:"rank"`
}

// GoalInfo is the active task's goal.
type GoalInfo struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Tasks  int    `json:"tasks"`
	Done   int    `json:"done"`
}

type ProjectInfo struct {
	Name    string `json:"name"`
	Context string `json:"context,omitempty"`
}
type TaskInfo struct {
	ID       string           `json:"id"`
	Title    string           `json:"title"`
	Status   string           `json:"status"`
	Profile  string           `json:"profile"`
	File     string           `json:"file"`
	Active   bool             `json:"active"`
	Open     []task.Criterion `json:"open_criteria,omitempty"`
	Criteria int              `json:"criteria"`
	Check    *task.Check      `json:"last_check,omitempty"`
	Failures int              `json:"consecutive_check_failures,omitempty"`
}
type TaskRef struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}
type HandoffInfo struct {
	At      string   `json:"at"`
	By      string   `json:"by"`
	Outcome string   `json:"outcome"`
	Summary string   `json:"summary"`
	Next    []string `json:"next,omitempty"`
	File    string   `json:"file"`
}

// Build assembles the brief for budget tokens. taskID optionally overrides the task.
func Build(p *project.Project, budget int, taskID string) *Brief {
	if budget <= 0 {
		budget = p.Config.BriefBudget()
	}
	b := &Brief{Budget: budget, Project: ProjectInfo{Name: p.Config.Project.Name, Context: contextLines(p, 3)}}
	tasks, _ := task.List(p)
	active := session.Load(p).Active()
	if taskID == "" {
		taskID = active
	}
	var t *task.Task
	for _, x := range tasks {
		if taskID != "" && strings.EqualFold(x.ID, taskID) {
			t = x
		}
	}
	if t == nil && taskID == "" {
		for _, x := range tasks {
			if x.Status == task.InProgress {
				t = x
				break
			}
		}
	}
	if t != nil {
		ti := &TaskInfo{ID: t.ID, Title: t.Title, Status: t.Status, Profile: t.Profile, File: t.File, Active: strings.EqualFold(t.ID, active)}
		cs := t.Criteria()
		ti.Criteria = len(cs)
		for _, c := range cs {
			if !c.Done {
				ti.Open = append(ti.Open, c)
			}
		}
		if t.Evidence != nil {
			ti.Check = t.Evidence.Check
			ti.Failures = t.Evidence.CheckFailures
		}
		b.Task = ti
	} else {
		for _, x := range tasks {
			if x.Status != task.Done && x.Status != task.Cancelled && len(b.Open) < 5 {
				b.Open = append(b.Open, TaskRef{x.ID, x.Title, x.Status})
			}
		}
		if len(tasks) == 0 {
			if lb, err := os.ReadFile(p.Path(project.LegacyDir, "current-task.md")); err == nil {
				b.Legacy = firstLines(string(lb), 40)
			}
		}
	}
	// Last handoff: for the task, else for the project.
	for _, h := range handoff.List(p) {
		if b.Task == nil || strings.EqualFold(h.Task, b.Task.ID) || h.Task == "" {
			b.Handoff = &HandoffInfo{At: h.At, By: h.By, Outcome: h.Outcome, Summary: strings.TrimSpace(h.Body), Next: h.Next, File: h.File}
			break
		}
	}
	if b.Handoff == nil && b.Task != nil {
		if hs := handoff.List(p); len(hs) > 0 {
			h := hs[0]
			b.Handoff = &HandoffInfo{At: h.At, By: h.By, Outcome: h.Outcome, Summary: strings.TrimSpace(h.Body), Next: h.Next, File: h.File}
		}
	}
	// Knowledge ranking: task tags + topics of changed paths + title words.
	base := ""
	if t != nil {
		base = t.Base
	}
	b.Changed = profile.DiffSince(p, base)
	b.Profile = profile.Compute(p, b.Changed)
	if t != nil && t.ProfilePinned && b.Profile != "strict" {
		b.Profile = t.Profile
	}
	var tags []string
	query := ""
	if t != nil {
		tags = append(tags, t.Tags...)
		query = t.Title
	}
	for _, c := range b.Changed {
		parts := strings.Split(c.Path, "/")
		query += " " + strings.Join(parts, " ")
	}
	b.Knowledge = knowledge.Rank(knowledge.LoadAll(p), query, tags, 15)
	if len(p.Config.Plugins.Enabled) > 0 {
		var taskRef any
		if b.Task != nil {
			taskRef = map[string]any{"id": b.Task.ID, "title": b.Task.Title}
		}
		paths := make([]string, 0, len(b.Changed))
		for _, c := range b.Changed {
			paths = append(paths, c.Path)
		}
		for _, r := range plugins.Call(p, "brief.sections", map[string]any{"changed_paths": paths, "budget": budget}, taskRef) {
			if r.Err != "" {
				b.PluginErr = append(b.PluginErr, r.Plugin+": "+r.Err)
				continue
			}
			var secs []PluginSection
			if json.Unmarshal(r.Data, &secs) != nil {
				b.PluginErr = append(b.PluginErr, r.Plugin+": brief.sections data is not a list of {title, body, rank}")
				continue
			}
			for _, s := range secs {
				if strings.TrimSpace(s.Body) != "" {
					s.Plugin = r.Plugin
					s.Body = textx.Truncate(s.Body, 2000)
					b.Plugins = append(b.Plugins, s)
				}
			}
		}
		sort.SliceStable(b.Plugins, func(i, j int) bool { return b.Plugins[i].Rank > b.Plugins[j].Rank })
	}
	if lines, unfilled, exists := conv.Lines(p, 12); exists {
		b.Conventions, b.ConventionsUnfilled = lines, unfilled
	}
	if t != nil && t.Goal != "" {
		b.Goal = goalInfo(p, t.Goal, tasks)
	}
	b.Rules = rules(b.Profile, p.Config.Check.Cmd != "")
	if active != "" && (t == nil || !strings.EqualFold(t.ID, active)) {
		b.Rules = append([]string{"The active task " + active + " does not exist on this branch (switched branches?): run `aitk doctor --fix`, then `aitk task next`."}, b.Rules...)
	}
	b.Next = next(b, p)
	// Fit the budget: drop knowledge from the lowest rank, then shorten handoff, then legacy.
	for {
		b.Markdown = render(b)
		b.Tokens = textx.Tokens(b.Markdown)
		if b.Tokens <= budget {
			break
		}
		switch {
		case len(b.Plugins) > 0:
			b.Plugins = b.Plugins[:len(b.Plugins)-1]
		case len(b.Knowledge) > 0:
			b.Knowledge = b.Knowledge[:len(b.Knowledge)-1]
			b.Dropped++
		case b.Handoff != nil && len([]rune(b.Handoff.Summary)) > 300:
			b.Handoff.Summary = textx.Truncate(b.Handoff.Summary, len([]rune(b.Handoff.Summary))/2)
		case len(b.Conventions) > 4:
			b.Conventions = b.Conventions[:len(b.Conventions)-1]
		case len([]rune(b.Legacy)) > 300:
			b.Legacy = textx.Truncate(b.Legacy, len([]rune(b.Legacy))/2)
		case len(b.Changed) > 10:
			b.Changed = b.Changed[:10]
		default:
			return b // irreducible sections exceed the budget; report honestly
		}
	}
	return b
}

func rank(p string) int { return map[string]int{"lite": 0, "standard": 1, "strict": 2}[p] }

func contextLines(p *project.Project, n int) string {
	raw, err := os.ReadFile(p.Path(project.ContextFile))
	if err != nil {
		return ""
	}
	var out []string
	inComment := false
	for _, l := range strings.Split(string(raw), "\n") {
		tl := strings.TrimSpace(l)
		if strings.HasPrefix(tl, "<!--") {
			inComment = !strings.Contains(tl, "-->")
			continue
		}
		if inComment {
			inComment = !strings.Contains(tl, "-->")
			continue
		}
		if tl == "" || strings.HasPrefix(tl, "# ") {
			continue
		}
		out = append(out, textx.Truncate(tl, 200))
		if len(out) == n {
			break
		}
	}
	return strings.Join(out, "\n")
}

func firstLines(s string, n int) string {
	ls := strings.Split(s, "\n")
	if len(ls) > n {
		ls = append(ls[:n], "…")
	}
	return strings.TrimSpace(strings.Join(ls, "\n"))
}

func rules(prof string, hasCheck bool) []string {
	r := []string{"Finish with: aitk close --summary \"…\" --knowledge \"…|none\"."}
	if hasCheck {
		r = append(r, "`aitk check` must pass on the final code (re-run after any change).")
	} else {
		r = append(r, "No check command is configured; verify your work manually and say how in --summary.")
	}
	switch prof {
	case "standard":
		r = append(r, "Every acceptance criterion must be satisfied: aitk task update <id> --ac-done N.")
	case "strict":
		r = append(r, "Every acceptance criterion must be satisfied: aitk task update <id> --ac-done N.",
			"Fill the task's Risk section: Impact, Security (STRIDE), Rollback, Validation.",
			"Done needs ≥2 review passes (last with 0 findings), one by a different tool: aitk review pass --findings N.")
	}
	r = append(r, "If unsure, blocked, or the check fails twice: stop and ask the user.")
	return r
}

func next(b *Brief, p *project.Project) []string {
	var n []string
	switch {
	case b.Task == nil && len(b.Open) > 0:
		n = append(n, "aitk task start "+b.Open[0].ID)
		n = append(n, `aitk task new "<title>" --ac "<criterion>"`)
	case b.Task == nil:
		n = append(n, `aitk task new "<title>" --ac "<criterion>"`, `aitk task start <id>`)
	case !b.Task.Active:
		n = append(n, "aitk task start "+b.Task.ID)
	}
	if b.Task != nil && b.Task.Active {
		if p.Config.Check.Cmd != "" {
			n = append(n, "aitk check")
		}
		if len(b.Task.Open) > 0 {
			n = append(n, fmt.Sprintf("aitk task update %s --ac-done 1", b.Task.ID))
		}
		n = append(n, `aitk close --summary "<what changed>" --knowledge "<finding|none>"`)
	}
	n = append(n, `aitk knowledge search "<topic>"`)
	if len(n) > 5 {
		n = n[:5]
	}
	return n
}

func render(b *Brief) string {
	var s strings.Builder
	fmt.Fprintf(&s, "# Brief: %s\n\n", b.Project.Name)
	if b.Project.Context != "" {
		s.WriteString(b.Project.Context + "\n\n")
	}
	s.WriteString("## Task\n")
	switch {
	case b.Task != nil:
		state := "not started in this worktree"
		if b.Task.Active {
			state = "active"
		}
		fmt.Fprintf(&s, "%s — %s\nStatus: %s (%s) · Profile: %s · File: %s\n", b.Task.ID, b.Task.Title, b.Task.Status, state, b.Task.Profile, b.Task.File)
		if b.Task.Criteria > 0 {
			fmt.Fprintf(&s, "Acceptance criteria open: %d of %d\n", len(b.Task.Open), b.Task.Criteria)
			for _, c := range b.Task.Open {
				s.WriteString("- [ ] " + c.Text + "\n")
			}
		} else if b.Task.Profile != "lite" {
			s.WriteString("No acceptance criteria yet (required before close).\n")
		}
		if g := b.Goal; g != nil {
			fmt.Fprintf(&s, "Goal: %s %s [%s] (%d/%d tasks done)\n", g.ID, g.Title, g.Status, g.Done, g.Tasks)
		}
		if c := b.Task.Check; c != nil {
			fmt.Fprintf(&s, "Last check: exit %d at %s\n", c.ExitCode, c.At)
		}
		if b.Task.Failures >= 2 {
			fmt.Fprintf(&s, "⚠ The check has failed %d times in a row. Do not keep retrying: ask the user, or hand over with aitk close --status blocked.\n", b.Task.Failures)
		}
	case len(b.Open) > 0:
		s.WriteString("No active task. Open tasks:\n")
		for _, t := range b.Open {
			fmt.Fprintf(&s, "- %s [%s] %s\n", t.ID, t.Status, t.Title)
		}
	case b.Legacy != "":
		s.WriteString("No structured tasks. Legacy current task (v1, docs/ai/_legacy/current-task.md):\n\n" + b.Legacy + "\n")
	default:
		s.WriteString("No tasks yet.\n")
	}
	if b.Handoff != nil {
		fmt.Fprintf(&s, "\n## Last handoff (%s, %s, %s)\n%s\n", b.Handoff.At, b.Handoff.By, b.Handoff.Outcome, b.Handoff.Summary)
		for _, n := range b.Handoff.Next {
			s.WriteString("- Next: " + n + "\n")
		}
	}
	if len(b.Changed) > 0 {
		fmt.Fprintf(&s, "\n## Changed files on this branch (profile: %s)\n", b.Profile)
		for _, c := range b.Changed {
			fmt.Fprintf(&s, "- %s (%d lines)\n", c.Path, c.Lines)
		}
	}
	if len(b.Conventions) > 0 || b.ConventionsUnfilled {
		s.WriteString("\n## Conventions (docs/ai/conventions.md)\n")
		for _, l := range b.Conventions {
			s.WriteString(l + "\n")
		}
		if b.ConventionsUnfilled {
			s.WriteString("Not written yet: ask the user, then fill docs/ai/conventions.md (stack, style, errors, data, tests, security).\n")
		}
	}
	if len(b.Knowledge) > 0 {
		s.WriteString("\n## Relevant knowledge\n")
		for _, k := range b.Knowledge {
			pin := ""
			if k.Pinned {
				pin = "📌 "
			}
			fmt.Fprintf(&s, "- %s%s (%s, %s)\n", pin, strings.ReplaceAll(k.Text, "\n", " "), k.Date, k.Topic)
		}
		if b.Dropped > 0 {
			fmt.Fprintf(&s, "(%d more omitted for budget: aitk knowledge search \"<topic>\")\n", b.Dropped)
		}
	}
	for _, ps := range b.Plugins {
		fmt.Fprintf(&s, "\n## %s (plugin %s)\n%s\n", ps.Title, ps.Plugin, strings.TrimSpace(ps.Body))
	}
	s.WriteString("\n## Rules\n")
	for _, r := range b.Rules {
		s.WriteString("- " + r + "\n")
	}
	s.WriteString("\n## Next commands\n")
	for _, n := range b.Next {
		s.WriteString("- `" + n + "`\n")
	}
	return s.String()
}

func goalInfo(p *project.Project, id string, tasks []*task.Task) *GoalInfo {
	raw, err := os.ReadFile(p.Path(project.StateFile))
	if err != nil {
		return nil
	}
	var st struct {
		Goals []struct{ ID, Title, Status string } `json:"goals"`
	}
	if json.Unmarshal(raw, &st) != nil {
		return nil
	}
	for _, g := range st.Goals {
		if strings.EqualFold(g.ID, id) {
			gi := &GoalInfo{ID: g.ID, Title: g.Title, Status: g.Status}
			for _, t := range tasks {
				if strings.EqualFold(t.Goal, g.ID) && t.Status != task.Cancelled {
					gi.Tasks++
					if t.Status == task.Done {
						gi.Done++
					}
				}
			}
			return gi
		}
	}
	return nil
}
