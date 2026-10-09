// Package doctor validates a project and repairs what is safe to repair.
package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/internal/agentsmd"
	"github.com/innaka-tech/ai-toolkit/internal/compat"
	"github.com/innaka-tech/ai-toolkit/internal/handoff"
	"github.com/innaka-tech/ai-toolkit/internal/knowledge"
	"github.com/innaka-tech/ai-toolkit/internal/plugins"
	"github.com/innaka-tech/ai-toolkit/internal/project"
	"github.com/innaka-tech/ai-toolkit/internal/schema"
	"github.com/innaka-tech/ai-toolkit/internal/session"
	"github.com/innaka-tech/ai-toolkit/internal/task"
)

// Check is one finding.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | error
	Detail string `json:"detail,omitempty"`
	Fixed  bool   `json:"fixed,omitempty"`
}

// Report is the doctor result.
type Report struct {
	Checks []Check `json:"checks"`
	Errors int     `json:"errors"`
	Warns  int     `json:"warnings"`
}

func (r *Report) add(name, status, detail string) *Check {
	r.Checks = append(r.Checks, Check{Name: name, Status: status, Detail: detail})
	switch status {
	case "error":
		r.Errors++
	case "warn":
		r.Warns++
	}
	return &r.Checks[len(r.Checks)-1]
}

// Run inspects p; with fix, it repairs generated files, the AGENTS.md block, and an overfull inbox.
func Run(p *project.Project, fix bool) *Report {
	r := &Report{}
	r.add("config", "ok", project.ConfigFile+" is valid")
	if p.Config.Check.Cmd == "" {
		r.add("check command", "warn", "no [check] cmd: the Definition of Done cannot verify work")
	}

	// state
	if b, err := os.ReadFile(p.Path(project.StateFile)); err != nil {
		r.add("state", "error", "ai-state.json missing (run: aitk init)")
	} else {
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			r.add("state", "error", "ai-state.json is not valid JSON: "+err.Error())
		} else if err := schema.Validate("state", v); err != nil {
			r.add("state", "error", err.Error())
		} else {
			r.add("state", "ok", "")
		}
	}

	// AGENTS.md
	b, _ := os.ReadFile(p.Path(project.AgentsFile))
	switch agentsmd.Inspect(string(b)) {
	case agentsmd.Current:
		r.add("AGENTS.md block", "ok", "")
	default:
		c := r.add("AGENTS.md block", "warn", "aitk block missing or outdated")
		if fix {
			if _, err := agentsmd.Sync(p.Path(project.AgentsFile)); err == nil {
				c.Fixed, c.Status = true, "ok"
				r.Warns--
			}
		}
	}
	if n := lines(p.Path(project.AgentsFile)); n > 120 {
		r.add("AGENTS.md size", "warn", fmt.Sprintf("%d lines (keep ≤ 120 so every agent reads it fully)", n))
	}
	if n := lines(p.Path(project.ContextFile)); n > 60 {
		r.add("project-context.md size", "warn", fmt.Sprintf("%d lines (keep ≤ 60)", n))
	}

	// tasks
	tasks, errs := task.List(p)
	for _, e := range errs {
		r.add("task file", "error", e.Error())
	}
	invalid := 0
	stale := []string{}
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour).Format("2006-01-02T15:04:05Z")
	for _, t := range tasks {
		if err := schema.Validate("task", t.Meta); err != nil {
			r.add("task "+t.ID, "error", err.Error())
			invalid++
		}
		if t.Status == task.InProgress && t.Updated < cutoff {
			stale = append(stale, t.ID)
		}
	}
	if invalid == 0 && len(errs) == 0 {
		r.add("tasks", "ok", fmt.Sprintf("%d valid", len(tasks)))
	}
	if len(stale) > 0 {
		r.add("stale tasks", "warn", "in_progress for over 7 days: "+strings.Join(stale, ", "))
	}
	if id := session.Load(p).Active(); id != "" {
		if _, err := task.Find(p, id); err != nil {
			c := r.add("session", "warn", "active task "+id+" no longer exists")
			if fix {
				s := session.Load(p)
				s.SetActive("", "", "")
				if session.Save(p, s) == nil {
					c.Fixed, c.Status = true, "ok"
					r.Warns--
				}
			}
		}
	}

	// handoffs
	bad := 0
	for _, h := range handoff.List(p) {
		if err := schema.Validate("handoff", h.Meta); err != nil {
			r.add("handoff "+h.File, "error", err.Error())
			bad++
		}
	}
	if bad == 0 {
		r.add("handoffs", "ok", "")
	}

	// knowledge
	long := 0
	for _, e := range knowledge.LoadAll(p) {
		if len([]rune(e.Text)) > 500 {
			long++
		}
	}
	if long > 0 {
		r.add("knowledge size", "warn", fmt.Sprintf("%d entries over 500 characters (aim for one or two sentences)", long))
	}
	if n := knowledge.InboxCount(p); n > p.Config.InboxLimit() {
		c := r.add("knowledge inbox", "warn", fmt.Sprintf("%d entries (limit %d)", n, p.Config.InboxLimit()))
		if fix {
			if _, err := knowledge.Compact(p, p.Config.ArchiveAfterDays(), false); err == nil {
				c.Fixed, c.Status = true, "ok"
				r.Warns--
			}
		}
	}
	if changed, _ := compat.EnsureGitattributes(p, true); changed {
		c := r.add(".gitattributes", "warn", "aitk merge rules missing (parallel branches may conflict)")
		if fix {
			if _, err := compat.EnsureGitattributes(p, false); err == nil {
				c.Fixed, c.Status = true, "ok"
				r.Warns--
			}
		}
	}
	if len(p.Config.Plugins.Enabled) > 0 {
		for _, res := range plugins.Call(p, "doctor.checks", nil, nil) {
			if res.Err != "" {
				r.add("plugin "+res.Plugin, "warn", res.Err)
				continue
			}
			var cs []Check
			json.Unmarshal(res.Data, &cs)
			for _, c := range cs {
				if c.Status != "ok" && c.Status != "warn" && c.Status != "error" {
					c.Status = "warn"
				}
				r.add("plugin "+res.Plugin+": "+c.Name, c.Status, c.Detail)
			}
		}
		for _, n := range p.Config.Plugins.Enabled {
			found := false
			for _, d := range plugins.Discover(p.Config) {
				found = found || d.Name == n
			}
			if !found {
				r.add("plugin "+n, "warn", "enabled in aitk.toml but aitk-"+n+" is not on PATH")
			}
		}
	}
	if fix {
		handoff.WriteIndex(p)
		compat.Write(p)
	}
	return r
}

func lines(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(b), "\n")
}
