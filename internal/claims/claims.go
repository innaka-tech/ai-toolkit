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

func path(p *project.Project) string { return filepath.Join(p.SharedDir(), "claims.json") }

// Load returns unexpired claims from every shared location (newest per task and worktree).
// Callers must hold the project lock to modify.
func Load(p *project.Project) []Claim {
	now := time.Now().UTC().Format(time.RFC3339)
	byKey := map[string]Claim{}
	var order []string
	for _, d := range p.SharedStateDirs() {
		var f file
		b, err := os.ReadFile(filepath.Join(d, "claims.json"))
		if err != nil || json.Unmarshal(b, &f) != nil {
			continue
		}
		for _, c := range f.Claims {
			if c.Expires <= now {
				continue
			}
			k := c.Task + "\x00" + c.Worktree
			if old, ok := byKey[k]; !ok || c.At > old.At {
				if !ok {
					order = append(order, k)
				}
				byKey[k] = c
			}
		}
	}
	out := make([]Claim, 0, len(order))
	for _, k := range order {
		out = append(out, byKey[k])
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
