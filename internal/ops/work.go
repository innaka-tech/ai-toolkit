package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/v2/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/v2/internal/brief"
	"github.com/innaka-tech/ai-toolkit/v2/internal/claims"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/handoff"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/session"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
)

// WorkResult describes the worktree prepared for a task.
type WorkResult struct {
	Task    string   `json:"task"`
	Path    string   `json:"path"`
	Branch  string   `json:"branch"`
	Created bool     `json:"created"`
	Copied  []string `json:"copied,omitempty"` // uncommitted aitk files copied into the worktree
}

// Work creates (or reuses) a worktree and branch dedicated to task id, claims the task, and starts it there.
func Work(p *project.Project, id string) (*WorkResult, error) {
	t, err := task.Find(p, id)
	if err != nil {
		return nil, apperr.TaskNotFound(id)
	}
	if h := claims.Holder(p, t.ID); h != nil {
		return nil, apperr.New("E_TASK_CLAIMED", apperr.ExitConflict, "pick another task (aitk task list)", "task %s is claimed by %s until %s", t.ID, h.By, h.Expires)
	}
	main := mainWorktree(p)
	path := filepath.Join(filepath.Dir(main), filepath.Base(main)+".aitk", t.ID)
	branch := "aitk/" + t.ID + "-" + project.Slug(t.Title, 30)
	res := &WorkResult{Task: t.ID, Path: path, Branch: branch}
	if !fsx.Exists(path) {
		if gitx.Head(p.Root) == "" {
			return nil, apperr.New("E_GIT", apperr.ExitRuntime, `git commit -m "chore: initial commit"`, "the repository has no commits yet")
		}
		args := []string{"worktree", "add", "-b", branch, path, "HEAD"}
		if _, err := gitx.Run(p.Root, "rev-parse", "--verify", "-q", "refs/heads/"+branch); err == nil {
			args = []string{"worktree", "add", path, branch}
		}
		if _, err := gitx.Run(p.Root, args...); err != nil {
			return nil, apperr.New("E_GIT", apperr.ExitRuntime, "git worktree list", "%v", err)
		}
		res.Created = true
	}
	// Bring over aitk files that are not committed yet, so the worktree sees the task.
	for _, rel := range []string{project.ConfigFile, project.StateFile, project.AgentsFile, t.File} {
		dst := filepath.Join(path, filepath.FromSlash(rel))
		if fsx.Exists(dst) {
			continue
		}
		if b, err := os.ReadFile(p.Path(rel)); err == nil {
			if err := fsx.WriteFile(dst, b, 0o644); err != nil {
				return nil, err
			}
			res.Copied = append(res.Copied, rel)
		}
	}
	wp, err := project.Open(path)
	if err != nil {
		return nil, err
	}
	if _, err := TaskStart(wp, t.ID); err != nil {
		return nil, err
	}
	return res, nil
}

func mainWorktree(p *project.Project) string {
	out, err := gitx.Run(p.Root, "worktree", "list", "--porcelain")
	if err == nil {
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "worktree ") {
				return strings.TrimPrefix(l, "worktree ")
			}
		}
	}
	return p.Root
}

// SwitchResult tells the caller how to hand the work to another tool.
type SwitchResult struct {
	Tool       string   `json:"tool"`
	Handoff    string   `json:"handoff"`
	Command    []string `json:"command,omitempty"` // argv to start the tool with the brief, when known
	PromptFile string   `json:"prompt_file"`
	Prompt     string   `json:"prompt"`
}

// toolCommands builds argv that starts a tool interactively with an initial prompt.
var toolCommands = map[string]func(prompt string) []string{
	"claude-code": func(s string) []string { return []string{"claude", s} },
	"codex":       func(s string) []string { return []string{"codex", s} },
	"gemini-cli":  func(s string) []string { return []string{"gemini", "-i", s} },
	"opencode":    func(s string) []string { return []string{"opencode", "--prompt", s} },
}

// SwitchTools lists tools whose start command aitk knows.
func SwitchTools() []string {
	var out []string
	for k := range toolCommands {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Switch writes a handoff to tool and prepares the prompt that continues the work there.
func Switch(p *project.Project, tool, note string) (*SwitchResult, error) {
	tool = normalizeTool(tool)
	var res *SwitchResult
	err := WithLock(p, func() error {
		s := session.Load(p)
		var next []string
		title := "Project"
		id := s.Active()
		if id != "" {
			if t, err := task.Find(p, id); err == nil {
				title = t.Title
				for i, c := range t.Criteria() {
					if !c.Done && len(next) < 10 {
						next = append(next, fmt.Sprintf("Satisfy criterion %d: %s", i+1, c.Text))
					}
				}
			}
		}
		if note == "" {
			note = "Handing over to " + tool + "."
		}
		h, err := handoff.Write(p, handoff.Meta{Task: id, At: task.Now(), By: Tool(), Outcome: "switched", To: tool, Next: next},
			"## "+title+"\n\n"+note+"\n")
		if err != nil {
			return err
		}
		res = &SwitchResult{Tool: tool, Handoff: h.File}
		return nil
	})
	if err != nil {
		return nil, err
	}
	b := brief.Build(p, 0, "")
	res.Prompt = "Continue the work in this repository. Read this aitk brief first and follow its rules; finish with `aitk close`.\n\n" + b.Markdown
	res.PromptFile = filepath.Join(p.AitkDir(), "switch-prompt.md")
	if err := fsx.WriteFile(res.PromptFile, []byte(res.Prompt), 0o644); err != nil {
		return nil, err
	}
	if f, ok := toolCommands[tool]; ok {
		res.Command = f(res.Prompt)
	}
	return res, nil
}

func normalizeTool(t string) string {
	switch strings.ToLower(t) {
	case "claude", "claude-code":
		return "claude-code"
	case "gemini", "gemini-cli":
		return "gemini-cli"
	}
	return project.Slug(t, 32)
}

// Report summarizes activity since a time (docs/spec/cli.md, report).
type Report struct {
	Since      string         `json:"since"`
	Done       []TaskRow      `json:"done"`
	InProgress []TaskRow      `json:"in_progress"`
	Blocked    []TaskRow      `json:"blocked"`
	InReview   []TaskRow      `json:"in_review"`
	ByTool     map[string]int `json:"handoffs_by_tool"`
	Handoffs   int            `json:"handoffs"`
	Knowledge  int            `json:"knowledge_added"`
}

// TaskRow is a report line.
type TaskRow struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Updated string `json:"updated"`
	By      string `json:"by,omitempty"`
	Commits int    `json:"commits,omitempty"`
}

// BuildReport aggregates tasks and handoffs updated since `since`.
func BuildReport(p *project.Project, since time.Time) *Report {
	cut := since.UTC().Format("2006-01-02T15:04:05Z")
	r := &Report{Since: cut, ByTool: map[string]int{}, Done: []TaskRow{}, InProgress: []TaskRow{}, Blocked: []TaskRow{}, InReview: []TaskRow{}}
	lastBy := map[string]string{}
	for _, h := range handoff.List(p) {
		if h.At < cut {
			continue
		}
		r.Handoffs++
		r.ByTool[h.By]++
		if _, ok := lastBy[h.Task]; !ok && h.Task != "" {
			lastBy[h.Task] = h.By
		}
	}
	tasks, _ := task.List(p)
	for _, t := range tasks {
		if t.Updated < cut {
			continue
		}
		row := TaskRow{ID: t.ID, Title: t.Title, Updated: t.Updated, By: lastBy[t.ID]}
		if t.Evidence != nil {
			row.Commits = len(t.Evidence.Commits)
		}
		switch t.Status {
		case task.Done:
			r.Done = append(r.Done, row)
		case task.InProgress, task.Implemented:
			r.InProgress = append(r.InProgress, row)
		case task.Blocked:
			r.Blocked = append(r.Blocked, row)
		case task.InReview:
			r.InReview = append(r.InReview, row)
		}
	}
	return r
}

// Markdown renders the report.
func (r *Report) Markdown(project string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Report: %s since %s\n", project, r.Since[:10])
	section := func(name string, rows []TaskRow) {
		fmt.Fprintf(&b, "\n## %s (%d)\n", name, len(rows))
		for _, x := range rows {
			by := ""
			if x.By != "" {
				by = " · " + x.By
			}
			fmt.Fprintf(&b, "- %s %s%s\n", x.ID, x.Title, by)
		}
	}
	section("Done", r.Done)
	section("In progress", r.InProgress)
	section("In review", r.InReview)
	section("Blocked", r.Blocked)
	fmt.Fprintf(&b, "\n## Sessions\n%d handoffs", r.Handoffs)
	var tools []string
	for k := range r.ByTool {
		tools = append(tools, k)
	}
	sort.Strings(tools)
	for _, k := range tools {
		fmt.Fprintf(&b, " · %s %d", k, r.ByTool[k])
	}
	b.WriteString("\n")
	return b.String()
}
