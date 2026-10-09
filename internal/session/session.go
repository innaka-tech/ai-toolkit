// Package session stores per-worktree state in the git directory (never committed).
package session

import (
	"encoding/json"
	"os"
	"path/filepath"

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
}

func path(p *project.Project) string { return filepath.Join(p.AitkDir(), "session.json") }

// Load returns the session, or an empty one.
func Load(p *project.Project) *Session {
	s := &Session{SchemaVersion: 1}
	b, err := os.ReadFile(path(p))
	if err == nil {
		json.Unmarshal(b, s)
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
