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
	"github.com/innaka-tech/ai-toolkit/v2/skills"
)

// Change is one file write an adapter wants.
type Change struct {
	Tool   string `json:"tool"`
	Path   string `json:"path"` // absolute
	Rel    string `json:"file"` // display path (repo-relative or ~/...)
	What   string `json:"what"`
	Global bool   `json:"global"`
	Delete bool   `json:"delete,omitempty"` // remove the file (aitk-owned files only)
	// problem, when set, means the change must not be applied: the reason is reported instead.
	problem string
	before  []byte
	after   []byte
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
	dirs    []string // or any of these project paths exists
	exts    []string // or an editor extension whose directory starts with one of these
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
		{name: "copilot", dirs: []string{".github/copilot-instructions.md"}, exts: []string{"github.copilot"}, project: copilotProject},
		{name: "windsurf", bins: []string{"windsurf"}, dirs: []string{".windsurf", ".windsurfrules"}, project: windsurfProject},
		{name: "cline", dirs: []string{".clinerules"}, exts: []string{"saoudrizwan.claude-dev"}, project: clineProject},
		{name: "roo", dirs: []string{".roo"}, exts: []string{"rooveterinaryinc.roo-cline"}, project: rooProject},
		{name: "aider", bins: []string{"aider"}, dirs: []string{".aider.conf.yml"}, project: aiderProject},
		{name: "junie", dirs: []string{".junie"}, project: junieProject},
		{name: "qwen-code", bins: []string{"qwen"}, dirs: []string{".qwen"}, project: qwenProject},
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
	for _, x := range a.exts {
		for _, editor := range []string{".vscode", ".vscode-insiders", ".cursor", ".windsurf", ".vscode-oss"} {
			if m, _ := filepath.Glob(filepath.Join(e.Home, editor, "extensions", x+"*")); len(m) > 0 {
				return true
			}
		}
	}
	return false
}

func pick(e Env, tools []string) ([]adapter, error) {
	if len(tools) == 1 && tools[0] == "all" { // every tool, for teams whose members use different ones
		return all(), nil
	}
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

// Pointer is the short instruction every tool-specific rule file carries: it sends the agent to
// AGENTS.md, where the full aitk block lives, so there is one source of truth.
const Pointer = "This repository uses aitk. Follow the \"Working in this repository (aitk)\" section of AGENTS.md:\n" +
	"run `aitk brief` first, take a task with `aitk task next --start` (or continue the active one), run `aitk check`\n" +
	"until it passes, then `aitk close --summary \"…\" --knowledge \"…|none\"`. If the aitk MCP tools are available, they do the same.\n"

const blockBegin, blockEnd = "<!-- aitk:pointer:begin -->", "<!-- aitk:pointer:end -->"

// findBlock locates the one aitk block in s. Missing markers are fine (no block yet); anything
// else that is not exactly one begin marker followed by one end marker is refused, because
// guessing could cut the user's own text.
func findBlock(s, begin, end string) (i, j int, found bool, problem string) {
	nb, ne := strings.Count(s, begin), strings.Count(s, end)
	switch {
	case nb == 0 && ne == 0:
		return 0, 0, false, ""
	case nb != 1 || ne != 1:
		return 0, 0, false, fmt.Sprintf("the aitk markers appear %d and %d times; fix the file by hand", nb, ne)
	}
	i, j = strings.Index(s, begin), strings.Index(s, end)
	if j < i {
		return 0, 0, false, "the aitk end marker comes before the begin marker; fix the file by hand"
	}
	return i, j, true, ""
}

// withBlock puts text between aitk markers in a hand-written file: replacing an earlier block,
// else appending. Everything else in the file stays as it is.
func withBlock(before []byte, begin, end, text string) ([]byte, string) {
	block := begin + "\n" + text + end + "\n"
	s := string(before)
	i, j, found, problem := findBlock(s, begin, end)
	if problem != "" {
		return before, problem
	}
	if found {
		return []byte(s[:i] + block + strings.TrimPrefix(s[j+len(end):], "\n")), ""
	}
	switch {
	case strings.TrimSpace(s) == "":
		return []byte(block), ""
	case strings.HasSuffix(s, "\n\n"):
		return []byte(s + block), ""
	case strings.HasSuffix(s, "\n"):
		return []byte(s + "\n" + block), ""
	}
	return []byte(s + "\n\n" + block), ""
}

// withoutBlock removes an aitk block.
func withoutBlock(before []byte, begin, end string) ([]byte, string) {
	s := string(before)
	i, j, found, problem := findBlock(s, begin, end)
	if problem != "" || !found {
		return before, problem
	}
	out := strings.TrimRight(s[:i], "\n")
	if rest := strings.TrimLeft(s[j+len(end):], "\n"); rest != "" {
		if out != "" {
			out += "\n\n"
		}
		out += rest
	}
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out), ""
}

// blockFile adds the pointer block to a markdown file that may be hand-written.
func blockFile(e Env, rel, what string) Change {
	p, before := projectFile(e, rel)
	after, problem := withBlock(before, blockBegin, blockEnd, Pointer)
	return Change{Path: p, Rel: rel, What: what, before: before, after: after, problem: problem}
}

// ruleFile writes an aitk-owned rule file (whole content).
func ruleFile(e Env, rel, what, content string) Change {
	p, before := projectFile(e, rel)
	return Change{Path: p, Rel: rel, What: what, before: before, after: []byte(content)}
}

// Plan computes the changes needed. Unchanged files are omitted. A problem with one tool's files
// (a JSONC config, an unreadable file, unbalanced markers) is an error when that one tool was
// asked for, and otherwise skips that change and is reported by PlanSkipping.
func Plan(e Env, tools []string, global bool) ([]Change, error) {
	cs, skipped, err := PlanSkipping(e, tools, global)
	if err == nil && len(skipped) > 0 && len(tools) == 1 && tools[0] != "all" {
		return nil, fmt.Errorf("%s", skipped[0])
	}
	return cs, err
}

// PlanSkipping is Plan that keeps going past problems and lists them.
func PlanSkipping(e Env, tools []string, global bool) ([]Change, []string, error) {
	as, err := pick(e, tools)
	if err != nil {
		return nil, nil, err
	}
	var out []Change
	var skipped []string
	for _, a := range as {
		cs, err := a.project(e)
		if err != nil {
			skipped = append(skipped, a.name+": "+err.Error())
			continue
		}
		if global && a.global != nil {
			g, err := a.global(e)
			if err != nil {
				skipped = append(skipped, a.name+": "+err.Error())
				continue
			}
			cs = append(cs, g...)
		}
		for _, c := range cs {
			if c.problem == "" {
				c.problem = unsafeTarget(c.Path)
			}
			if c.problem != "" {
				skipped = append(skipped, a.name+": "+c.Rel+": "+c.problem)
				continue
			}
			if !bytes.Equal(c.before, c.after) || c.Delete && len(c.before) > 0 {
				c.Tool = a.name
				out = append(out, c)
			}
		}
	}
	return out, skipped, nil
}

// unsafeTarget explains why a file must not be written: it exists but cannot be read (writing
// would lose content that could not even be backed up), it is read-only, or it is a symlink
// whose target is missing.
func unsafeTarget(path string) string {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		return err.Error()
	}
	if info.Mode()&os.ModeSymlink != 0 {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "symlink to a missing file; fix the link first"
		}
		if info, err = os.Stat(real); err != nil {
			return err.Error()
		}
	}
	if info.IsDir() {
		return "is a directory"
	}
	if info.Mode().Perm()&0o200 == 0 {
		return "read-only; make it writable to let aitk edit it"
	}
	if _, err := os.ReadFile(path); err != nil {
		return "cannot be read (" + err.Error() + "); left alone"
	}
	return ""
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
		if c.Delete {
			if info, err := os.Lstat(c.Path); err == nil && info.Mode()&os.ModeSymlink != 0 {
				// A linked dotfile: empty the target instead of breaking the link.
				if err := fsx.WriteFileKeep(c.Path, nil, 0o644); err != nil {
					return err
				}
				continue
			}
			if err := os.Remove(c.Path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if err := fsx.WriteFileKeep(c.Path, c.after, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Configured lists tools whose project files already contain aitk entries.
func Configured(e Env) []string {
	var out []string
	for _, a := range all() {
		cs, err := a.project(e)
		if err != nil {
			continue
		}
		for _, c := range cs {
			if bytes.Contains(c.before, []byte("aitk")) {
				out = append(out, a.name)
				break
			}
		}
	}
	return out
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
	sp, sb := projectFile(e, ".claude/skills/aitk/SKILL.md")
	cs = append(cs, Change{Path: sp, Rel: ".claude/skills/aitk/SKILL.md", What: "Agent Skill: aitk workflow", before: sb, after: skills.AitkSkill})
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
	return geminiLike(e, ".gemini/settings.json", "GEMINI.md", true)
}

// qwenProject: Qwen Code shares Gemini CLI's settings format.
func qwenProject(e Env) ([]Change, error) {
	return geminiLike(e, ".qwen/settings.json", "QWEN.md", false)
}

func geminiLike(e Env, rel, own string, hook bool) ([]Change, error) {
	p, _ := projectFile(e, rel)
	what := "MCP server, AGENTS.md as context"
	if hook {
		what += ", SessionStart hook"
	}
	c, err := jsonChange(p, rel, what, false, func(o *jsonedit.Object) error {
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
			if !contains(names, own) {
				names = append(names, own)
			}
			if err := ctx.Set("fileName", names); err != nil {
				return err
			}
			if err := o.Set("context", ctx); err != nil {
				return err
			}
		}
		if !hook {
			return nil
		}
		return ensureSessionHook("startup")(o)
	})
	return []Change{c}, err
}

// copilotProject: GitHub Copilot reads .github/copilot-instructions.md (and AGENTS.md in its
// coding agent); VS Code reads workspace MCP servers from .vscode/mcp.json.
func copilotProject(e Env) ([]Change, error) {
	p, _ := projectFile(e, ".vscode/mcp.json")
	c, err := jsonChange(p, ".vscode/mcp.json", "register aitk MCP server (VS Code)", false,
		setServer("servers", map[string]any{"type": "stdio", "command": mcpCommand[0], "args": mcpCommand[1:]}))
	if err != nil {
		return nil, err
	}
	return []Change{blockFile(e, ".github/copilot-instructions.md", "instructions: follow AGENTS.md"), c}, nil
}

func windsurfProject(e Env) ([]Change, error) {
	return []Change{ruleFile(e, ".windsurf/rules/aitk.md", "always-on rule: follow AGENTS.md", "---\ntrigger: always_on\n---\n\n# aitk\n\n"+Pointer)}, nil
}

// clineProject: a .clinerules directory gets its own file; a single .clinerules file gets a block.
func clineProject(e Env) ([]Change, error) {
	if info, err := os.Stat(filepath.Join(e.Root, ".clinerules")); err == nil && !info.IsDir() {
		return []Change{blockFile(e, ".clinerules", "rule: follow AGENTS.md")}, nil
	}
	return []Change{ruleFile(e, ".clinerules/aitk.md", "rule: follow AGENTS.md", "# aitk\n\n"+Pointer)}, nil
}

func rooProject(e Env) ([]Change, error) {
	p, _ := projectFile(e, ".roo/mcp.json")
	c, err := jsonChange(p, ".roo/mcp.json", "register aitk MCP server", false, setServer("mcpServers", stdioEntry()))
	if err != nil {
		return nil, err
	}
	return []Change{ruleFile(e, ".roo/rules/aitk.md", "rule: follow AGENTS.md", "# aitk\n\n"+Pointer), c}, nil
}

func junieProject(e Env) ([]Change, error) {
	return []Change{blockFile(e, ".junie/guidelines.md", "guidelines: follow AGENTS.md")}, nil
}

// aiderProject makes Aider read AGENTS.md in every session (read: in .aider.conf.yml). A file
// that already sets read: is left alone unless it lists AGENTS.md; doctor then shows it pending.
func aiderProject(e Env) ([]Change, error) {
	p, before := projectFile(e, ".aider.conf.yml")
	after := before
	s := string(before)
	switch {
	case strings.Contains(s, "AGENTS.md"):
	case !hasYAMLKey(s, "read"):
		sep := ""
		if s != "" && !strings.HasSuffix(s, "\n") {
			sep = "\n"
		}
		after = []byte(s + sep + "# aitk: every session reads the aitk workflow\nread: [AGENTS.md]\n")
	}
	return []Change{{Path: p, Rel: ".aider.conf.yml", What: "read AGENTS.md in every session", before: before, after: after}}, nil
}

func hasYAMLKey(s, key string) bool {
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, key+":") {
			return true
		}
	}
	return false
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
	i, j, found, problem := findBlock(s, tomlBegin, tomlEnd)
	if problem != "" {
		return []Change{{Path: p, Rel: "~/.codex/config.toml", What: "register aitk MCP server", Global: true, before: before, after: before, problem: problem}}, nil
	}
	switch {
	case found:
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
	if c.Delete {
		return "delete " + c.Rel
	}
	if len(c.before) == 0 {
		return "create " + c.Rel
	}
	return "update " + c.Rel
}
