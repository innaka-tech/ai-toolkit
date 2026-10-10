// Package doctor validates a project and repairs what is safe to repair.
package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/v2/internal/adapters"
	"github.com/innaka-tech/ai-toolkit/v2/internal/agentsmd"
	"github.com/innaka-tech/ai-toolkit/v2/internal/audit"
	"github.com/innaka-tech/ai-toolkit/v2/internal/compat"
	"github.com/innaka-tech/ai-toolkit/v2/internal/conv"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/handoff"
	"github.com/innaka-tech/ai-toolkit/v2/internal/heal"
	"github.com/innaka-tech/ai-toolkit/v2/internal/knowledge"
	"github.com/innaka-tech/ai-toolkit/v2/internal/plugins"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/schema"
	"github.com/innaka-tech/ai-toolkit/v2/internal/session"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
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
	// Self-healing: damaged aitk files (merge conflicts, hand-edited task frontmatter).
	if fix {
		for _, a := range heal.Run(p) {
			r.Checks = append(r.Checks, Check{Name: "heal " + a.File, Status: "ok", Detail: a.Action + ": " + a.Detail, Fixed: a.Action != "quarantined"})
			if a.Action == "quarantined" {
				r.Checks[len(r.Checks)-1].Status = "warn"
				r.Warns++
			}
		}
	} else {
		for _, a := range heal.Problems(p) {
			r.add("heal "+a.File, "warn", "repairable with aitk doctor --fix: "+a.Detail)
		}
	}

	if p.Config.Check.Cmd == "" && project.DetectCheck(p.Root) == "" {
		r.add("check command", "warn", "no [check] cmd: the Definition of Done cannot verify work")
	}

	// state
	if b, err := os.ReadFile(p.Path(project.StateFile)); err != nil {
		r.add("state", "error", "ai-state.json missing (run: aitk init)")
	} else {
		var v any
		bad := ""
		if err := json.Unmarshal(b, &v); err != nil {
			bad = "ai-state.json is not valid JSON: " + err.Error()
		} else if err := schema.Validate("state", v); err != nil {
			bad = "ai-state.json: " + err.Error()
		}
		switch {
		case bad == "":
			r.add("state", "ok", "")
		case fix && restoreState(p):
			c := r.add("state", "ok", bad+"; restored the last committed version (the broken copy is in "+project.LegacyDir+"/quarantine/)")
			c.Fixed = true
		default:
			r.add("state", "error", bad+" (fix: git checkout -- ai-state.json, or repair it by hand; doctor --fix restores it only when a valid version is committed)")
		}
	}

	// AGENTS.md
	b, _ := os.ReadFile(p.Path(project.AgentsFile))
	switch agentsmd.Inspect(string(b)) {
	case agentsmd.Newer:
		r.add("AGENTS.md block", "warn", "aitk block from a newer aitk release: upgrade aitk (the block is left as it is)")
	case agentsmd.Current:
		r.add("AGENTS.md block", "ok", "")
	default:
		why := map[agentsmd.State]string{agentsmd.Outdated: "from an older aitk release", agentsmd.Edited: "edited by hand", agentsmd.Missing: "missing", agentsmd.V1Only: "still the v1 protocol", agentsmd.Newer: "from a newer aitk release: upgrade aitk"}[agentsmd.Inspect(string(b))]
		c := r.add("AGENTS.md block", "warn", "aitk block "+why+" (doctor --fix replaces only the block)")
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
		if _, err := task.Find(p, id); err != nil && !taskFileExists(p, id) {
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
	// Adapters that drifted from what aitk installs.
	env := adapters.DefaultEnv(p.Root)
	if tools := adapters.Configured(env); len(tools) > 0 {
		if cs, err := adapters.Plan(env, tools, false); err == nil && len(cs) > 0 {
			var files []string
			for _, c := range cs {
				files = append(files, c.Rel)
			}
			c := r.add("adapters", "warn", "drifted: "+strings.Join(files, ", "))
			if fix && adapters.Apply(cs, "") == nil {
				c.Fixed, c.Status = true, "ok"
				r.Warns--
			}
		}
	}
	// A missing check command when one can be detected.
	if p.Config.Check.Cmd == "" {
		if cmd := project.DetectCheck(p.Root); cmd != "" {
			c := r.add("check command", "warn", "not configured; detected: "+cmd)
			if fix && addCheck(p, cmd) == nil {
				c.Fixed, c.Status, c.Detail = true, "ok", "set to "+cmd
				r.Warns--
			}
		}
	}
	if _, unfilled, exists := conv.Lines(p, 1); !exists {
		c := r.add("conventions", "warn", conv.File+" is missing: agents have no coding conventions to follow")
		if fix && fsx.WriteFile(p.Path(conv.File), []byte(conv.Template), 0o644) == nil {
			c.Fixed, c.Status, c.Detail = true, "warn", "created "+conv.File+" from the template; fill it in"
		}
	} else if unfilled {
		r.add("conventions", "warn", conv.File+" is still the template: fill in the stack, style, errors, data, tests, and security rules")
	}
	if p.Config.Security.Audit != "off" && len(p.Config.Security.Scanners) == 0 && audit.HasManifest(p.Root) {
		if _, missing := audit.Detect(p); len(missing) > 0 {
			r.add("security scanners", "warn", "none installed for "+strings.Join(missing, ", ")+": aitk audit cannot check dependency vulnerabilities (fix: aitk audit install)")
		}
	}
	if hw := compat.HandWritten(p); len(hw) > 0 {
		r.add("generated indexes", "warn", "left untouched because they are hand-written: "+strings.Join(hw, ", ")+" (move them to docs/ai/_legacy/ to let aitk maintain them)")
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

// addCheck appends a [check] table to aitk.toml when none exists.
func addCheck(p *project.Project, cmd string) error {
	path := p.Path(project.ConfigFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) == "[check]" {
			return fmt.Errorf("[check] already present")
		}
	}
	s := strings.TrimRight(string(b), "\n") + fmt.Sprintf("\n\n[check]\ncmd = %q\ntimeout = \"10m\"\n", cmd)
	return fsx.WriteFileKeep(path, []byte(s), 0o644)
}

// taskFileExists reports whether a file for task id exists, even if it cannot be parsed
// (a damaged file must not make aitk forget the active task).
func taskFileExists(p *project.Project, id string) bool {
	m, _ := filepath.Glob(p.Path(project.TasksDir, id+"-*.md"))
	return len(m) > 0
}

// restoreState puts back the committed ai-state.json when that version is valid, keeping the
// broken copy in quarantine.
func restoreState(p *project.Project) bool {
	head, err := gitx.RunRaw(p.Root, "show", "HEAD:"+project.StateFile)
	if err != nil {
		return false
	}
	var v any
	if json.Unmarshal(head, &v) != nil || schema.Validate("state", v) != nil {
		return false
	}
	broken, _ := os.ReadFile(p.Path(project.StateFile))
	q := p.Path(project.LegacyDir, "quarantine", "ai-state.json."+time.Now().UTC().Format("20060102T150405Z"))
	if fsx.WriteFile(q, broken, 0o644) != nil {
		return false
	}
	return fsx.WriteFile(p.Path(project.StateFile), head, 0o644) == nil
}
