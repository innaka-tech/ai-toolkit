// Package task reads and writes task files (docs/spec/workflow.md §4).
package task

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/innaka-tech/ai-toolkit/v2/internal/doc"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/schema"
)

// Statuses.
const (
	Todo        = "todo"
	InProgress  = "in_progress"
	Implemented = "implemented"
	InReview    = "in_review"
	Done        = "done"
	Blocked     = "blocked"
	Cancelled   = "cancelled"
)

// Statuses lists valid statuses.
var Statuses = []string{Todo, InProgress, Implemented, InReview, Done, Blocked, Cancelled}

// IDPattern matches task IDs (ADR-0009).
var IDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,31}$`)

// Check is evidence of one check run.
type Check struct {
	Cmd        string `yaml:"cmd" json:"cmd"`
	ExitCode   int    `yaml:"exit_code" json:"exit_code"`
	DurationMs int64  `yaml:"duration_ms,omitempty" json:"duration_ms,omitempty"`
	Summary    string `yaml:"summary,omitempty" json:"summary,omitempty"`
	At         string `yaml:"at" json:"at"`
	Tree       string `yaml:"tree,omitempty" json:"tree,omitempty"`
}

// Pass is one review pass.
type Pass struct {
	N        int    `yaml:"n" json:"n"`
	By       string `yaml:"by" json:"by"`
	Session  string `yaml:"session,omitempty" json:"session,omitempty"`
	At       string `yaml:"at" json:"at"`
	Findings int    `yaml:"findings" json:"findings"`
}

// Review holds bug-hunt passes.
type Review struct {
	Passes []Pass `yaml:"passes,omitempty" json:"passes,omitempty"`
}

// Acceptance counts criteria.
type Acceptance struct {
	Total int `yaml:"total" json:"total"`
	Done  int `yaml:"done" json:"done"`
}

// Evidence is written by check and close.
type Evidence struct {
	Check      *Check      `yaml:"check,omitempty" json:"check,omitempty"`
	Commits    []string    `yaml:"commits,omitempty" json:"commits,omitempty"`
	Acceptance *Acceptance `yaml:"acceptance,omitempty" json:"acceptance,omitempty"`
}

// Meta is the frontmatter (schemas/task.schema.json).
type Meta struct {
	ID            string         `yaml:"id" json:"id"`
	Title         string         `yaml:"title" json:"title"`
	Status        string         `yaml:"status" json:"status"`
	Profile       string         `yaml:"profile" json:"profile"`
	ProfilePinned bool           `yaml:"profile_pinned,omitempty" json:"profile_pinned,omitempty"`
	Goal          string         `yaml:"goal,omitempty" json:"goal,omitempty"`
	Tags          []string       `yaml:"tags,omitempty" json:"tags,omitempty"`
	DependsOn     []string       `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Created       string         `yaml:"created" json:"created"`
	Updated       string         `yaml:"updated" json:"updated"`
	Closed        string         `yaml:"closed,omitempty" json:"closed,omitempty"`
	CreatedBy     string         `yaml:"created_by,omitempty" json:"created_by,omitempty"`
	Branch        string         `yaml:"branch,omitempty" json:"branch,omitempty"`
	Paths         []string       `yaml:"paths,omitempty" json:"paths,omitempty"`
	Review        *Review        `yaml:"review,omitempty" json:"review,omitempty"`
	Evidence      *Evidence      `yaml:"evidence,omitempty" json:"evidence,omitempty"`
	Legacy        map[string]any `yaml:"legacy,omitempty" json:"legacy,omitempty"`
}

// Task is a parsed task file.
type Task struct {
	Meta
	Body string `yaml:"-" json:"body,omitempty"`
	File string `yaml:"-" json:"file"` // repository-relative
}

// Now returns the current UTC time in the canonical format.
func Now() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

// Load reads one task file.
func Load(p *project.Project, rel string) (*Task, error) {
	b, err := os.ReadFile(p.Path(rel))
	if err != nil {
		return nil, err
	}
	front, body, ok := doc.Split(string(b))
	if !ok {
		return nil, fmt.Errorf("%s: missing YAML frontmatter", rel)
	}
	t := &Task{Body: body, File: rel}
	if err := yaml.Unmarshal([]byte(front), &t.Meta); err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	return t, nil
}

// List loads every task, sorted by updated (newest first). Unreadable files are returned in errs.
func List(p *project.Project) (tasks []*Task, errs []error) {
	entries, _ := os.ReadDir(p.Path(project.TasksDir))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		t, err := Load(p, filepath.ToSlash(filepath.Join(project.TasksDir, e.Name())))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		tasks = append(tasks, t)
	}
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].Updated > tasks[j].Updated })
	return tasks, errs
}

// Find returns the task with id.
func Find(p *project.Project, id string) (*Task, error) {
	tasks, _ := List(p)
	for _, t := range tasks {
		if strings.EqualFold(t.ID, id) {
			return t, nil
		}
	}
	return nil, os.ErrNotExist
}

// Save validates and writes the task atomically.
func Save(p *project.Project, t *Task) error {
	if err := schema.Validate("task", t.Meta); err != nil {
		return err
	}
	b, err := doc.Join(t.Meta, t.Body)
	if err != nil {
		return err
	}
	if t.File == "" {
		t.File = filepath.ToSlash(filepath.Join(project.TasksDir, t.ID+"-"+project.Slug(t.Title, 48)+".md"))
	}
	return fsx.WriteFile(p.Path(t.File), b, 0o644)
}

const crockford = "0123456789abcdefghjkmnpqrstvwxyz"

// NewID returns an unused random ID: T- plus 4+ Crockford base32 characters.
func NewID(p *project.Project) string {
	used := map[string]bool{}
	tasks, _ := List(p)
	for _, t := range tasks {
		used[strings.ToLower(t.ID)] = true
	}
	for n := 4; ; n++ {
		for attempt := 0; attempt < 8; attempt++ {
			buf := make([]byte, n)
			rand.Read(buf)
			for i := range buf {
				buf[i] = crockford[int(buf[i])%32]
			}
			id := "T-" + string(buf)
			if !used[strings.ToLower(id)] {
				return id
			}
		}
	}
}

// Criterion is one acceptance criterion line.
type Criterion struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

var acLine = regexp.MustCompile(`^\s*[-*] \[( |x|X)\] (.+)$`)

// Criteria parses the "Acceptance criteria" section.
func (t *Task) Criteria() []Criterion {
	sec, ok := doc.Section(t.Body, "Acceptance criteria")
	if !ok {
		return nil
	}
	var out []Criterion
	for _, l := range strings.Split(sec, "\n") {
		if m := acLine.FindStringSubmatch(l); m != nil {
			out = append(out, Criterion{Text: m[2], Done: m[1] != " "})
		}
	}
	return out
}

// SetCriteria rewrites the "Acceptance criteria" section.
func (t *Task) SetCriteria(cs []Criterion) {
	var b strings.Builder
	for _, c := range cs {
		mark := " "
		if c.Done {
			mark = "x"
		}
		fmt.Fprintf(&b, "- [%s] %s\n", mark, c.Text)
	}
	t.Body = doc.SetSection(t.Body, "Acceptance criteria", b.String())
}

// NewBody renders the body template for a new task.
func NewBody(objective string, criteria []string) string {
	var b strings.Builder
	b.WriteString("## Objective\n")
	if objective == "" {
		objective = "(describe the outcome in one paragraph)"
	}
	b.WriteString(objective + "\n\n## Acceptance criteria\n")
	for _, c := range criteria {
		b.WriteString("- [ ] " + c + "\n")
	}
	b.WriteString("\n## Risk\n" + RiskPlaceholder + "\n\n## Notes\n")
	return b.String()
}

// RiskPlaceholder marks an unfilled risk section.
const RiskPlaceholder = "(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)"

// RiskFilled reports whether the Risk section contains real analysis.
func (t *Task) RiskFilled() bool {
	sec, ok := doc.Section(t.Body, "Risk")
	if !ok {
		return false
	}
	s := strings.TrimSpace(strings.ReplaceAll(sec, RiskPlaceholder, ""))
	if len(s) < 40 {
		return false
	}
	low := strings.ToLower(s)
	for _, k := range []string{"impact", "security", "rollback", "validation"} {
		if !strings.Contains(low, k) {
			return false
		}
	}
	return true
}
