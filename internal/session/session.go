// Package session stores per-worktree state in the git directory (never committed).
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/innaka-tech/ai-toolkit/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/internal/project"
	"github.com/innaka-tech/ai-toolkit/internal/task"
)

// Session mirrors schemas/session.schema.json.
type Session struct {
	SchemaVersion int         `json:"schema_version"`
	ActiveTask    *string     `json:"active_task"`
	StartedAt     string      `json:"started_at,omitempty"`
	Tool          string      `json:"tool,omitempty"`
	LastCheck     *task.Check `json:"last_check,omitempty"`
	LastTask      string      `json:"last_task,omitempty"`
	LastTaskAt    string      `json:"last_task_at,omitempty"`
}

func path(p *project.Project) string { return filepath.Join(p.AitkDir(), "session.json") }

// Load returns the most recently written session among the state locations (a sandboxed
// tool may only be able to write the fallback), or an empty one.
func Load(p *project.Project) *Session {
	s := &Session{SchemaVersion: 1}
	var newest time.Time
	for _, d := range p.StateDirs() {
		f := filepath.Join(d, "session.json")
		info, err := os.Stat(f)
		if err != nil || !info.ModTime().After(newest) {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var x Session
		if json.Unmarshal(b, &x) == nil {
			*s, newest = x, info.ModTime()
		}
	}
	s.SchemaVersion = 1
	return s
}

// Save writes the session.
func Save(p *project.Project, s *Session) error {
	b, _ := json.MarshalIndent(s, "", "  ")
	return fsx.WriteFile(path(p), append(b, '\n'), 0o644)
}

// Active returns the active task ID, or "".
func (s *Session) Active() string {
	if s.ActiveTask == nil {
		return ""
	}
	return *s.ActiveTask
}

// SetActive sets or clears the active task.
func (s *Session) SetActive(id, at, tool string) {
	if id == "" {
		s.ActiveTask, s.StartedAt = nil, ""
		return
	}
	s.ActiveTask, s.StartedAt, s.Tool = &id, at, tool
}

// CommitTask is the task a commit belongs to: the active one, else one closed in the last hour
// (agents usually commit right after closing).
func (s *Session) CommitTask(now time.Time) string {
	if a := s.Active(); a != "" {
		return a
	}
	if s.LastTask != "" {
		if t, err := time.Parse(time.RFC3339, s.LastTaskAt); err == nil && now.Sub(t) < time.Hour {
			return s.LastTask
		}
	}
	return ""
}
