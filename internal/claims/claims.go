// Package claims records which worktree is working on which task. Claims live in the git
// directory shared by all worktrees (never committed) and expire after a TTL.
package claims

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/innaka-tech/ai-toolkit/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/internal/project"
)

// Claim of a task by a worktree.
type Claim struct {
	Task     string `json:"task"`
	By       string `json:"by"`
	Worktree string `json:"worktree"` // per-worktree git dir
	At       string `json:"at"`
	Expires  string `json:"expires"`
}

type file struct {
	SchemaVersion int     `json:"schema_version"`
	Claims        []Claim `json:"claims"`
}

func path(p *project.Project) string { return filepath.Join(p.CommonDir, "aitk", "claims.json") }

// Load returns unexpired claims. Callers must hold the project lock to modify.
func Load(p *project.Project) []Claim {
	var f file
	b, err := os.ReadFile(path(p))
	if err == nil {
		json.Unmarshal(b, &f)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var out []Claim
	for _, c := range f.Claims {
		if c.Expires > now {
			out = append(out, c)
		}
	}
	return out
}

func save(p *project.Project, cs []Claim) error {
	if cs == nil {
		cs = []Claim{}
	}
	b, _ := json.MarshalIndent(file{SchemaVersion: 1, Claims: cs}, "", "  ")
	return fsx.WriteFile(path(p), append(b, '\n'), 0o644)
}

// Holder returns the live claim on task by another worktree, if any.
func Holder(p *project.Project, task string) *Claim {
	for _, c := range Load(p) {
		if c.Task == task && c.Worktree != p.GitDir {
			c := c
			return &c
		}
	}
	return nil
}

// Take claims task for this worktree (renewing an own claim). It does not check holders.
func Take(p *project.Project, task, by string, ttl time.Duration) (Claim, error) {
	now := time.Now().UTC()
	c := Claim{Task: task, By: by, Worktree: p.GitDir, At: now.Format(time.RFC3339), Expires: now.Add(ttl).Format(time.RFC3339)}
	var keep []Claim
	for _, x := range Load(p) {
		if !(x.Task == task && x.Worktree == p.GitDir) {
			keep = append(keep, x)
		}
	}
	return c, save(p, append(keep, c))
}

// Release drops this worktree's claim on task (or any claim with force).
func Release(p *project.Project, task string, force bool) (bool, error) {
	var keep []Claim
	released := false
	for _, x := range Load(p) {
		if x.Task == task && (force || x.Worktree == p.GitDir) {
			released = true
			continue
		}
		keep = append(keep, x)
	}
	return released, save(p, keep)
}
