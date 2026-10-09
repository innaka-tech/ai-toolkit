package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/schema"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
)

// ---------- goals ----------

// Goal mirrors ai-state.json goals.
type Goal struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Parent string `json:"parent,omitempty"`
	Tasks  int    `json:"tasks"`
	Done   int    `json:"done"`
}

func readState(p *project.Project) (map[string]any, error) {
	b, err := os.ReadFile(p.Path(project.StateFile))
	if err != nil {
		return nil, apperr.New("E_SCHEMA", apperr.ExitGate, "aitk doctor --fix", "ai-state.json: %v", err)
	}
	var st map[string]any
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, apperr.New("E_SCHEMA", apperr.ExitGate, "aitk doctor", "ai-state.json is not valid JSON")
	}
	return st, nil
}

func writeState(p *project.Project, st map[string]any) error {
	if err := schema.Validate("state", st); err != nil {
		return apperr.New("E_SCHEMA", apperr.ExitGate, "check the goal fields", "%v", err)
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	return fsx.WriteFile(p.Path(project.StateFile), append(b, '\n'), 0o644)
}

// Goals lists goals with task progress.
func Goals(p *project.Project) []Goal {
	st, err := readState(p)
	if err != nil {
		return nil
	}
	raw, _ := st["goals"].([]any)
	tasks, _ := task.List(p)
	var out []Goal
	for _, r := range raw {
		m, _ := r.(map[string]any)
		g := Goal{ID: str(m["id"]), Title: str(m["title"]), Status: str(m["status"]), Parent: str(m["parent"])}
		for _, t := range tasks {
			if strings.EqualFold(t.Goal, g.ID) && t.Status != task.Cancelled {
				g.Tasks++
				if t.Status == task.Done {
					g.Done++
				}
			}
		}
		out = append(out, g)
	}
	return out
}

func str(v any) string { s, _ := v.(string); return s }

// FindGoal returns the goal with id.
func FindGoal(p *project.Project, id string) *Goal {
	for _, g := range Goals(p) {
		if strings.EqualFold(g.ID, id) {
			g := g
			return &g
		}
	}
	return nil
}

var goalNum = regexp.MustCompile(`^G-(\d+)$`)

// GoalAdd creates a goal (status planned).
func GoalAdd(p *project.Project, title, id, parent string) (*Goal, error) {
	title = strings.TrimSpace(title)
	if len([]rune(title)) < 3 {
		return nil, apperr.Usage("goal title must be at least 3 characters")
	}
	if err := GuardText("goal", title); err != nil {
		return nil, err
	}
	var g *Goal
	err := WithLock(p, func() error {
		st, err := readState(p)
		if err != nil {
			return err
		}
		raw, _ := st["goals"].([]any)
		used := map[string]bool{}
		max := 0
		for _, r := range raw {
			gid := str(r.(map[string]any)["id"])
			used[strings.ToLower(gid)] = true
			if m := goalNum.FindStringSubmatch(gid); m != nil {
				if n, _ := strconv.Atoi(m[1]); n > max {
					max = n
				}
			}
		}
		if id == "" {
			id = fmt.Sprintf("G-%d", max+1)
		}
		if used[strings.ToLower(id)] {
			return apperr.Usage("goal %s already exists", id)
		}
		if parent != "" && !used[strings.ToLower(parent)] {
			return apperr.New("E_USAGE", apperr.ExitUsage, "aitk goal list", "parent goal %s does not exist", parent)
		}
		entry := map[string]any{"id": id, "title": title, "status": "planned"}
		if parent != "" {
			entry["parent"] = parent
		}
		st["goals"] = append(raw, entry)
		if err := writeState(p, st); err != nil {
			return err
		}
		g = &Goal{ID: id, Title: title, Status: "planned", Parent: parent}
		return nil
	})
	return g, err
}

// GoalStatus changes a goal's status.
func GoalStatus(p *project.Project, id, status string) (*Goal, error) {
	switch status {
	case "planned", "active", "achieved", "dropped":
	default:
		return nil, apperr.Usage("invalid goal status %q (planned, active, achieved, dropped)", status)
	}
	err := WithLock(p, func() error {
		st, err := readState(p)
		if err != nil {
			return err
		}
		raw, _ := st["goals"].([]any)
		for _, r := range raw {
			m := r.(map[string]any)
			if strings.EqualFold(str(m["id"]), id) {
				m["status"] = status
				return writeState(p, st)
			}
		}
		return apperr.New("E_USAGE", apperr.ExitUsage, "aitk goal list", "goal %s not found", id)
	})
	if err != nil {
		return nil, err
	}
	return FindGoal(p, id), nil
}
