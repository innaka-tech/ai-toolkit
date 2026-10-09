package ops

import (
	"sort"
	"strconv"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/claims"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/session"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
)

// NextFilter narrows which tasks are candidates.
type NextFilter struct {
	Goal string
	Tag  string
	Skip map[string]bool // task IDs to leave out (e.g. already attempted in this run)
}

// Candidate is a task that could be worked on next, or why it cannot.
type Candidate struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Profile   string   `json:"profile"`
	WaitingOn []string `json:"waiting_on,omitempty"` // unfinished dependencies
	ClaimedBy string   `json:"claimed_by,omitempty"` // another worktree's claim
}

// NextResult lists ready tasks in the order they should be worked on, and tasks waiting on others.
type NextResult struct {
	Ready   []Candidate `json:"ready"`
	Waiting []Candidate `json:"waiting"`
}

// OpenDependencies returns the dependencies of t that are not done or cancelled (missing IDs count as open).
func OpenDependencies(t *task.Task, byID map[string]*task.Task) []string {
	var open []string
	for _, d := range t.DependsOn {
		dt := byID[strings.ToLower(d)]
		if dt == nil || (dt.Status != task.Done && dt.Status != task.Cancelled) {
			open = append(open, d)
		}
	}
	return open
}

// Next orders the work that can start now: this worktree's active task, then unfinished tasks
// nobody else holds (in_progress before todo), oldest first. Tasks waiting for a person (in_review),
// blocked, done, or cancelled are not candidates; tasks with open dependencies are listed as waiting.
func Next(p *project.Project, f NextFilter) *NextResult {
	tasks, _ := task.List(p)
	byID := map[string]*task.Task{}
	for _, t := range tasks {
		byID[strings.ToLower(t.ID)] = t
	}
	active := session.Load(p).Active()
	held := map[string]string{}
	for _, c := range claims.Load(p) {
		if c.Worktree != p.GitDir {
			held[c.Task] = c.By
		}
	}
	sort.SliceStable(tasks, func(i, j int) bool { return before(tasks[i], tasks[j], active) })
	res := &NextResult{Ready: []Candidate{}, Waiting: []Candidate{}}
	for _, t := range tasks {
		if t.Status != task.Todo && t.Status != task.InProgress || f.Skip[t.ID] {
			continue
		}
		if f.Goal != "" && !strings.EqualFold(t.Goal, f.Goal) || f.Tag != "" && !contains(t.Tags, f.Tag) {
			continue
		}
		c := Candidate{ID: t.ID, Title: t.Title, Status: t.Status, Profile: t.Profile}
		c.WaitingOn = OpenDependencies(t, byID)
		c.ClaimedBy = held[t.ID]
		if len(c.WaitingOn) > 0 || c.ClaimedBy != "" {
			res.Waiting = append(res.Waiting, c)
			continue
		}
		res.Ready = append(res.Ready, c)
	}
	return res
}

func before(a, b *task.Task, active string) bool {
	if ra, rb := strings.EqualFold(a.ID, active), strings.EqualFold(b.ID, active); ra != rb {
		return ra
	}
	if (a.Status == task.InProgress) != (b.Status == task.InProgress) {
		return a.Status == task.InProgress
	}
	if a.Created != b.Created {
		return a.Created < b.Created
	}
	return naturalLess(a.ID, b.ID)
}

// naturalLess compares IDs so that "X-T9" sorts before "X-T10".
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		da, db := digitsPrefix(a), digitsPrefix(b)
		if da != "" && db != "" {
			na, _ := strconv.Atoi(da)
			nb, _ := strconv.Atoi(db)
			if na != nb {
				return na < nb
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func digitsPrefix(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' && i < 9 {
		i++
	}
	return s[:i]
}
