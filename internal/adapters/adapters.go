// Package adapters installs aitk into AI coding tools (docs/spec/integrations.md):
// instructions, MCP registration, and session hooks. Project files are committed with
// the repository; global files change only with --global and are backed up first.
package adapters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit"
)

// Change is one file write an adapter wants.
type Change struct {
	Tool   string `json:"tool"`
	Path   string `json:"path"` // absolute
	Rel    string `json:"file"` // display path (repo-relative or ~/...)
	What   string `json:"what"`
	Global bool   `json:"global"`
	before []byte
	after  []byte
}

// Status of an adapter in doctor output.
type Status struct {
	Tool      string   `json:"tool"`
	Installed bool     `json:"installed"`
	Current   bool     `json:"current"`
	Pending   []string `json:"pending,omitempty"`
}

// Env abstracts the machine for tests.
type Env struct {
	Root     string // project root
	Home     string
	LookPath func(string) (string, error)
}

// DefaultEnv uses the real machine.
func DefaultEnv(root string) Env {
	home, _ := os.UserHomeDir()
	return Env{Root: root, Home: home, LookPath: exec.LookPath}
}

type adapter struct {
	name    string
	bins    []string // any of these on PATH means installed
	dirs    []string // or any of these project dirs exists
	project func(e Env) ([]Change, error)
	global  func(e Env) ([]Change, error)
}

// MCPCommand is how tools launch the server; "aitk" must be on PATH.
var mcpCommand = []string{"aitk", "mcp"}

const briefHook = "aitk brief"

func all() []adapter {
	return []adapter{
		{name: "claude-code", bins: []string{"claude"}, dirs: []string{".claude"}, project: claudeProject},
		{name: "codex", bins: []string{"codex"}, project: noop, global: codexGlobal},
		{name: "opencode", bins: []string{"opencode"}, project: opencodeProject},
		{name: "gemini-cli", bins: []string{"gemini"}, dirs: []string{".gemini"}, project: geminiProject},
		{name: "kiro", bins: []string{"kiro", "kiro-cli"}, dirs: []string{".kiro"}, project: kiroProject},
		{name: "cursor", bins: []string{"cursor", "cursor-agent"}, dirs: []string{".cursor"}, project: cursorProject},
	}
}

// Names lists supported tools.
func Names() []string {
	var n []string
	for _, a := range all() {
		n = append(n, a.name)
	}
	return n
}

func (a adapter) installed(e Env) bool {
	for _, b := range a.bins {
		if _, err := e.LookPath(b); err == nil {
			return true
		}
	}
	for _, d := range a.dirs {
		if fsx.Exists(filepath.Join(e.Root, d)) {
			return true
		}
	}
	return false
}

func pick(e Env, tools []string) ([]adapter, error) {
	if len(tools) == 0 {
		var out []adapter
		for _, a := range all() {
			if a.installed(e) {
				out = append(out, a)
			}
		}
		return out, nil
	}
	var out []adapter
	for _, t := range tools {
		found := false
		for _, a := range all() {
			if a.name == t {
				out = append(out, a)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown tool %q (supported: %s)", t, strings.Join(Names(), ", "))
		}
	}
	return out, nil
}

// Plan computes the changes needed. Unchanged files are omitted.
func Plan(e Env, tools []string, global bool) ([]Change, error) {
	as, err := pick(e, tools)
	if err != nil {
		return nil, err
	}
	var out []Change
	for _, a := range as {
		cs, err := a.project(e)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", a.name, err)
		}
		if global && a.global != nil {
			g, err := a.global(e)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", a.name, err)
			}
			cs = append(cs, g...)
		}
		for _, c := range cs {
			if !bytes.Equal(c.before, c.after) {
				c.Tool = a.name
				out = append(out, c)
			}
		}
	}
	return out, nil
}

// Apply writes the changes. Global files are backed up to backupDir first.
func Apply(cs []Change, backupDir string) error {
	stamp := time.Now().UTC().Format("20060102T150405Z")
	for _, c := range cs {
		if c.Global && len(c.before) > 0 {
			dst := filepath.Join(backupDir, stamp, strings.TrimPrefix(filepath.ToSlash(c.Rel), "~/"))
			if err := fsx.WriteFile(dst, c.before, 0o600); err != nil {
				return fmt.Errorf("backup %s: %w", c.Rel, err)
			}
		}
		if err := fsx.WriteFile(c.Path, c.after, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Doctor reports per-tool state without writing.
func Doctor(e Env, global bool) []Status {
	var out []Status
	for _, a := range all() {
		s := Status{Tool: a.name, Installed: a.installed(e)}
		if s.Installed {
			cs, err := Plan(e, []string{a.name}, global)
			if err != nil {
				s.Pending = []string{err.Error()}
			}
			for _, c := range cs {
				s.Pending = append(s.Pending, c.Rel+": "+c.What)
			}
			s.Current = err == nil && len(cs) == 0
		}
		out = append(out, s)
	}
	return out
}

// ---- per-tool ----

func noop(Env) ([]Change, error) { return nil, nil }

func read(path string) []byte { b, _ := os.ReadFile(path); return b }

func projectFile(e Env, rel string) (string, []byte) {
	p := filepath.Join(e.Root, filepath.FromSlash(rel))
	return p, read(p)
}

// jsonChange edits a JSON file with fn and returns the change.
func jsonChange(path, rel, what string, global bool, fn func(o *jsonedit.Object) error) (Change, error) {
	before := read(path)
	o, err := jsonedit.Parse(before)
	if err != nil {
		return Change{}, fmt.Errorf("%s is not plain JSON (%v); add the aitk entry by hand", rel, err)
	}
	if err := fn(o); err != nil {
		return Change{}, err
	}
	after := o.Bytes()
	if semanticallyEqual(before, after) {
		after = before
	}
	return Change{Path: path, Rel: rel, What: what, Global: global, before: before, after: after}, nil
}

func semanticallyEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya)
}

// setServer sets <container>.aitk = entry.
func setServer(container string, entry any) func(o *jsonedit.Object) error {
	return func(o *jsonedit.Object) error {
		c, err := o.Child(container)
		if err != nil {
			return err
		}
		if err := c.Set("aitk", entry); err != nil {
			return err
		}
		return o.Set(container, c)
	}
}

// ensureSessionHook adds a SessionStart command hook running `aitk brief`, once.
func ensureSessionHook(matcher string) func(o *jsonedit.Object) error {
	return func(o *jsonedit.Object) error {
		hooks, err := o.Child("hooks")
		if err != nil {
			return err
		}
		var groups []map[string]any
		if raw, ok := hooks.Get("SessionStart"); ok {
			if err := json.Unmarshal(raw, &groups); err != nil {
				return fmt.Errorf("hooks.SessionStart: %w", err)
			}
		}
		for _, g := range groups {
			hs, _ := g["hooks"].([]any)
			for _, h := range hs {
				if m, _ := h.(map[string]any); m != nil {
					if cmd, _ := m["command"].(string); strings.HasPrefix(cmd, briefHook) {
						return nil // already installed
					}
				}
			}
		}
		groups = append(groups, map[string]any{"matcher": matcher, "hooks": []any{map[string]any{"type": "command", "command": briefHook}}})
		if err := hooks.Set("SessionStart", groups); err != nil {
			return err
		}
		return o.Set("hooks", hooks)
	}
}

func stdioEntry() map[string]any {
	return map[string]any{"command": mcpCommand[0], "args": mcpCommand[1:]}
}

func claudeProject(e Env) ([]Change, error) {
	var cs []Change
	p, _ := projectFile(e, ".mcp.json")
	c, err := jsonChange(p, ".mcp.json", "register aitk MCP server", false, setServer("mcpServers", stdioEntry()))
	if err != nil {
		return nil, err
	}
	cs = append(cs, c)
	p, _ = projectFile(e, ".claude/settings.json")
	c, err = jsonChange(p, ".claude/settings.json", "SessionStart hook: aitk brief", false, ensureSessionHook("startup|resume|compact"))
	if err != nil {
		return nil, err
	}
	cs = append(cs, c)
	p, before := projectFile(e, "CLAUDE.md")
	after := before
	if !bytes.Contains(before, []byte("@AGENTS.md")) {
		if len(bytes.TrimSpace(before)) == 0 {
			after = []byte("@AGENTS.md\n")
		} else {
			after = append([]byte("@AGENTS.md\n\n"), before...)
		}
	}
	cs = append(cs, Change{Path: p, Rel: "CLAUDE.md", What: "import AGENTS.md", before: before, after: after})
	return cs, nil
}

func opencodeProject(e Env) ([]Change, error) {
	p, _ := projectFile(e, "opencode.json")
	c, err := jsonChange(p, "opencode.json", "register aitk MCP server", false, func(o *jsonedit.Object) error {
		if _, ok := o.Get("$schema"); !ok {
			if err := o.Set("$schema", "https://opencode.ai/config.json"); err != nil {
				return err
			}
		}
		return setServer("mcp", map[string]any{"type": "local", "command": mcpCommand, "enabled": true})(o)
	})
	return []Change{c}, err
}

func geminiProject(e Env) ([]Change, error) {
	p, _ := projectFile(e, ".gemini/settings.json")
	c, err := jsonChange(p, ".gemini/settings.json", "MCP server, AGENTS.md as context, SessionStart hook", false, func(o *jsonedit.Object) error {
		if err := setServer("mcpServers", stdioEntry())(o); err != nil {
			return err
		}
		ctx, err := o.Child("context")
		if err != nil {
			return err
		}
		var names []string
		if raw, ok := ctx.Get("fileName"); ok {
			var one string
			if json.Unmarshal(raw, &names) != nil && json.Unmarshal(raw, &one) == nil {
				names = []string{one}
			}
		}
		if !contains(names, "AGENTS.md") {
			names = append([]string{"AGENTS.md"}, names...)
			if !contains(names, "GEMINI.md") {
				names = append(names, "GEMINI.md")
			}
			if err := ctx.Set("fileName", names); err != nil {
				return err
			}
			if err := o.Set("context", ctx); err != nil {
				return err
			}
		}
		return ensureSessionHook("startup")(o)
	})
	return []Change{c}, err
}

const steering = `---
inclusion: always
---

# aitk

This repository uses aitk. Follow the "Working in this repository (aitk)" section of AGENTS.md:
run ` + "`aitk brief`" + ` first, work on one task, run ` + "`aitk check`" + ` until it passes, then ` + "`aitk close`" + `.
`

func kiroProject(e Env) ([]Change, error) {
	p, _ := projectFile(e, ".kiro/settings/mcp.json")
	c, err := jsonChange(p, ".kiro/settings/mcp.json", "register aitk MCP server", false, setServer("mcpServers", stdioEntry()))
	if err != nil {
		return nil, err
	}
	sp, sb := projectFile(e, ".kiro/steering/aitk.md")
	return []Change{c, {Path: sp, Rel: ".kiro/steering/aitk.md", What: "steering: follow AGENTS.md", before: sb, after: []byte(steering)}}, nil
}

const cursorRule = `---
description: aitk workflow
alwaysApply: true
---

Follow the "Working in this repository (aitk)" section of AGENTS.md: run ` + "`aitk brief`" + ` first, work on one task,
run ` + "`aitk check`" + ` until it passes, then ` + "`aitk close`" + `.
`

func cursorProject(e Env) ([]Change, error) {
	p, _ := projectFile(e, ".cursor/mcp.json")
	c, err := jsonChange(p, ".cursor/mcp.json", "register aitk MCP server", false, setServer("mcpServers", stdioEntry()))
	if err != nil {
		return nil, err
	}
	rp, rb := projectFile(e, ".cursor/rules/aitk.mdc")
	return []Change{c, {Path: rp, Rel: ".cursor/rules/aitk.mdc", What: "always-on rule: follow AGENTS.md", before: rb, after: []byte(cursorRule)}}, nil
}

const tomlBegin, tomlEnd = "# aitk:begin", "# aitk:end"

// codexGlobal registers the server in ~/.codex/config.toml (Codex has no project MCP config).
func codexGlobal(e Env) ([]Change, error) {
	p := filepath.Join(e.Home, ".codex", "config.toml")
	before := read(p)
	block := tomlBegin + "\n[mcp_servers.aitk]\ncommand = \"aitk\"\nargs = [\"mcp\"]\n" + tomlEnd + "\n"
	s := string(before)
	var after string
	switch i, j := strings.Index(s, tomlBegin), strings.Index(s, tomlEnd); {
	case i >= 0 && j > i:
		after = s[:i] + block + strings.TrimPrefix(s[j+len(tomlEnd):], "\n")
	case strings.Contains(s, "[mcp_servers.aitk]"):
		after = s // configured by hand; leave it
	default:
		sep := "\n"
		if s == "" || strings.HasSuffix(s, "\n\n") {
			sep = ""
		} else if !strings.HasSuffix(s, "\n") {
			sep = "\n\n"
		}
		after = s + sep + block
	}
	return []Change{{Path: p, Rel: "~/.codex/config.toml", What: "register aitk MCP server", Global: true, before: before, after: []byte(after)}}, nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Diff renders a short before/after summary for dry runs.
func Diff(c Change) string {
	if len(c.before) == 0 {
		return "create " + c.Rel
	}
	return "update " + c.Rel
}
