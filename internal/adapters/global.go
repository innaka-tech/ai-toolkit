package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit"
	"github.com/innaka-tech/ai-toolkit/v2/internal/slash"
	"github.com/innaka-tech/ai-toolkit/v2/skills"
)

// GlobalBegin and GlobalEnd delimit aitk's block in a tool's user-level instructions.
const GlobalBegin, GlobalEnd = "<!-- aitk:global:begin -->", "<!-- aitk:global:end -->"

// GlobalText tells an agent, in any repository on this machine, how to recognise an aitk project
// and what to do there. It is deliberately short: the repository's AGENTS.md has the details.
const GlobalText = `## aitk (applies in any repository that has aitk.toml at its root)

A repository with ` + "`aitk.toml`" + ` is managed by aitk. In such a repository, without being asked:
1. Start every session with ` + "`aitk brief`" + ` (or the aitk MCP tool ` + "`brief`" + `) and follow what it says.
2. Take work with ` + "`aitk task next --start`" + ` or continue the active task; run ` + "`aitk check`" + ` until it passes;
   run ` + "`aitk impact`" + ` before finishing.
3. Finish the task yourself: mark each satisfied criterion with ` + "`aitk task update <id> --ac-done N`" + `, then run
   ` + "`aitk close --summary \"…\" --knowledge \"…|none\"`" + `. That is how work becomes done; you do not need to ask first.
   If close refuses, do what its fix line says.
4. aitk's rules take precedence over older ai-toolkit instructions (ai-start, ai-close, editing docs/ai/*.md by hand).
5. Never edit task status or evidence by hand and never accept user acceptance (UAT) for the user; when stuck,
   ` + "`aitk close --status blocked`" + ` and stop.
In a repository without aitk.toml, ignore this section.
`

// SetupResult reports a setup run.
type SetupResult struct {
	Changes []Change `json:"changes"`
	Skipped []string `json:"skipped,omitempty"` // targets that could not be edited safely, with why
}

type globalTarget struct {
	tool  string
	bins  []string
	homes []string // home-relative paths whose existence means the tool is used
	build func(e Env, remove bool) ([]Change, error)
}

func globalTargets() []globalTarget {
	return []globalTarget{
		{"claude-code", []string{"claude"}, []string{".claude"}, func(e Env, rm bool) ([]Change, error) {
			cs := []Change{blockChange(e, ".claude/CLAUDE.md", "user instructions: recognise aitk projects", rm)}
			// A user-level Agent Skill: Claude Code loads it in every project and uses it when the
			// repository is an aitk project (its description says so).
			cs = append(cs, ownedFile(e, ".claude/skills/aitk/SKILL.md", "user-level Agent Skill: aitk workflow", skills.AitkSkill, rm))
			cs = append(cs, globalCmds(e, ".claude/commands", "claude", rm)...)
			return append(cs, claudeUserMCP(e, rm)...), nil
		}},
		{"codex", []string{"codex"}, []string{".codex"}, func(e Env, rm bool) ([]Change, error) {
			cs := []Change{blockChange(e, ".codex/AGENTS.md", "user instructions: recognise aitk projects", rm)}
			cs = append(cs, globalCmds(e, ".codex/prompts", "claude", rm)...) // Codex custom prompts: /prompts:aitk-…
			cs = append(cs, ownedFile(e, ".codex/skills/aitk/SKILL.md", "user-level Agent Skill: aitk workflow", skills.AitkSkill, rm))
			mcp, err := codexMCP(e, rm)
			return append(cs, mcp...), err
		}},
		{"opencode", []string{"opencode"}, []string{".config/opencode"}, func(e Env, rm bool) ([]Change, error) {
			cs := []Change{blockChange(e, ".config/opencode/AGENTS.md", "user instructions: recognise aitk projects", rm)}
			cs = append(cs, globalCmds(e, ".config/opencode/commands", "opencode", rm)...)
			cs = append(cs, commandChanges(e.Home, ".config/opencode/command", "~/.config/opencode/command", slash.Formats["opencode"], nil, true, true)...) // aitk ≤ 2.6.0 used the singular folder
			if fsx.Exists(filepath.Join(e.Home, ".config/opencode/opencode.jsonc")) {
				return cs, fmt.Errorf("MCP not registered: ~/.config/opencode/opencode.jsonc has comments; add the aitk server by hand")
			}
			c, err := globalJSON(e, ".config/opencode/opencode.json", "register aitk MCP server", "mcp",
				map[string]any{"type": "local", "command": mcpCommand, "enabled": true}, rm)
			if err != nil {
				return cs, err
			}
			return append(cs, c), nil
		}},
		{"gemini-cli", []string{"gemini"}, []string{".gemini"}, func(e Env, rm bool) ([]Change, error) {
			cs := []Change{blockChange(e, ".gemini/GEMINI.md", "user instructions: recognise aitk projects", rm)}
			cs = append(cs, globalCmds(e, ".gemini/commands", "gemini", rm)...)
			c, err := globalJSON(e, ".gemini/settings.json", "register aitk MCP server", "mcpServers", stdioEntry(), rm)
			if err != nil {
				return cs, err
			}
			return append(cs, c), nil
		}},
		{"qwen-code", []string{"qwen"}, []string{".qwen"}, func(e Env, rm bool) ([]Change, error) {
			cs := []Change{blockChange(e, ".qwen/QWEN.md", "user instructions: recognise aitk projects", rm)}
			return append(cs, globalCmds(e, ".qwen/commands", "gemini", rm)...), nil
		}},
		{"kiro", []string{"kiro", "kiro-cli"}, []string{".kiro"}, func(e Env, rm bool) ([]Change, error) {
			return []Change{ownedFile(e, ".kiro/steering/aitk.md", "global steering: recognise aitk projects",
				[]byte("---\ninclusion: always\n---\n\n"+GlobalText), rm)}, nil
		}},
		{"cursor", []string{"cursor", "cursor-agent"}, []string{".cursor"}, func(e Env, rm bool) ([]Change, error) {
			return globalCmds(e, ".cursor/commands", "cursor", rm), nil
		}},
		{"antigravity", []string{"agy"}, []string{".gemini/antigravity-cli"}, func(e Env, rm bool) ([]Change, error) {
			cs := []Change{blockChange(e, ".gemini/antigravity-cli/AGENTS.md", "user instructions: recognise aitk projects", rm)}
			cs = append(cs, ownedFile(e, ".gemini/antigravity-cli/skills/aitk/SKILL.md", "Agent Skill: aitk workflow", skills.AitkSkill, rm))
			return append(cs, cliMCP(e, "agy", rm, []string{"agy", "mcp", "list"}, []string{"agy", "mcp", "add", "aitk", "aitk", "mcp"}, []string{"agy", "mcp", "remove", "aitk"})...), nil
		}},
		{"jcode", []string{"jcode"}, []string{".jcode"}, func(e Env, rm bool) ([]Change, error) {
			cs := []Change{blockChange(e, ".jcode/prompt-overlay.md", "user instructions: recognise aitk projects", rm)}
			c, err := globalJSON(e, ".jcode/mcp.json", "register aitk MCP server", "servers",
				map[string]any{"command": mcpCommand[0], "args": mcpCommand[1:], "env": map[string]any{}, "shared": true}, rm)
			if err != nil {
				return cs, err
			}
			return append(cs, c), nil
		}},
		{"pi", []string{"pi"}, []string{".pi/agent"}, func(e Env, rm bool) ([]Change, error) {
			// pi has no built-in MCP support (by design); it uses aitk through its shell tool.
			cs := []Change{blockChange(e, ".pi/agent/AGENTS.md", "user instructions: recognise aitk projects", rm)}
			cs = append(cs, ownedFile(e, ".pi/agent/skills/aitk/SKILL.md", "Agent Skill: aitk workflow", skills.AitkSkill, rm))
			return append(cs, globalCmds(e, ".pi/agent/prompts", "pi", rm)...), nil
		}},
		{"windsurf", []string{"windsurf"}, []string{".codeium/windsurf"}, func(e Env, rm bool) ([]Change, error) {
			return []Change{blockChange(e, ".codeium/windsurf/memories/global_rules.md", "global rules: recognise aitk projects", rm)}, nil
		}},
	}
}

// Setup computes the user-level changes that make every AI tool on this machine recognise aitk
// projects without being told: a marked block in each tool's own user instructions, the Agent
// Skill for Claude Code, and the MCP server where the tool has a user-level MCP config. With
// remove, it computes the changes that take all of that out again. Tools that are not installed
// are left alone, unless named in tools.
func Setup(e Env, tools []string, remove bool) (*SetupResult, error) {
	res := &SetupResult{Changes: []Change{}}
	known := map[string]bool{}
	for _, g := range globalTargets() {
		known[g.tool] = true
	}
	for _, t := range tools {
		if !known[t] {
			var names []string
			for _, g := range globalTargets() {
				names = append(names, g.tool)
			}
			return nil, fmt.Errorf("unknown tool %q (setup supports: %s)", t, strings.Join(names, ", "))
		}
	}
	for _, g := range globalTargets() {
		if len(tools) > 0 && !contains(tools, g.tool) {
			continue
		}
		if len(tools) == 0 && !g.used(e) {
			continue
		}
		cs, err := g.build(e, remove)
		if err != nil {
			res.Skipped = append(res.Skipped, g.tool+": "+err.Error())
		}
		for _, c := range cs {
			if c.problem == "" && len(c.Run) == 0 {
				c.problem = unsafeTarget(c.Path)
			}
			if c.problem != "" {
				res.Skipped = append(res.Skipped, g.tool+": "+c.Rel+": "+c.problem)
				continue
			}
			if len(c.Run) > 0 || !bytes.Equal(c.before, c.after) || c.Delete && len(c.before) > 0 {
				c.Tool, c.Global = g.tool, true
				res.Changes = append(res.Changes, c)
			}
		}
	}
	return res, nil
}

func (g globalTarget) used(e Env) bool {
	for _, b := range g.bins {
		if _, err := e.LookPath(b); err == nil {
			return true
		}
	}
	for _, h := range g.homes {
		if fsx.Exists(filepath.Join(e.Home, h)) {
			return true
		}
	}
	return false
}

func homeFile(e Env, rel string) (string, []byte) {
	p := filepath.Join(e.Home, filepath.FromSlash(rel))
	return p, read(p)
}

func blockChange(e Env, rel, what string, remove bool) Change {
	p, before := homeFile(e, rel)
	after, problem := withBlock(before, GlobalBegin, GlobalEnd, GlobalText)
	if remove {
		after, problem = withoutBlock(before, GlobalBegin, GlobalEnd)
		what = "remove: " + what
		if problem == "" && len(bytes.TrimSpace(after)) == 0 && len(before) > 0 { // the file held only aitk's block
			return Change{Path: p, Rel: "~/" + rel, What: what, before: before, Delete: true}
		}
	}
	return Change{Path: p, Rel: "~/" + rel, What: what, before: before, after: after, problem: problem}
}

// ownedFile is a file aitk writes whole; remove deletes it (only when it is still aitk's).
func ownedFile(e Env, rel, what string, content []byte, remove bool) Change {
	p, before := homeFile(e, rel)
	if remove {
		if len(before) == 0 || !bytes.Contains(before, []byte("aitk")) {
			return Change{Path: p, Rel: "~/" + rel, before: before, after: before}
		}
		return Change{Path: p, Rel: "~/" + rel, What: "remove: " + what, before: before, Delete: true}
	}
	if len(before) > 0 && !bytes.Contains(before, []byte("aitk")) {
		return Change{Path: p, Rel: "~/" + rel, What: what, before: before, after: content, problem: "a file of yours with this name exists; left alone"}
	}
	return Change{Path: p, Rel: "~/" + rel, What: what, before: before, after: content}
}

func globalJSON(e Env, rel, what, container string, entry any, remove bool) (Change, error) {
	p, _ := homeFile(e, rel)
	if remove {
		c, err := jsonChange(p, "~/"+rel, "remove: "+what, true, func(o *jsonedit.Object) error {
			raw, ok := o.Get(container)
			if !ok || !bytes.Contains(raw, []byte(`"aitk"`)) {
				return nil
			}
			c, err := o.Child(container)
			if err != nil {
				return err
			}
			c.Delete("aitk")
			if c.Len() == 0 {
				o.Delete(container)
				return nil
			}
			return o.Set(container, c)
		})
		if err == nil && len(c.before) > 0 && bytes.Equal(bytes.TrimSpace(c.after), []byte("{}")) {
			c.Delete = true // the file held only aitk's entry
		}
		return c, err
	}
	return jsonChange(p, "~/"+rel, what, true, setServer(container, entry))
}

func codexMCP(e Env, remove bool) ([]Change, error) {
	if !remove {
		return codexGlobal(e)
	}
	p, before := homeFile(e, ".codex/config.toml")
	after := before
	s := string(before)
	if i, j := strings.Index(s, tomlBegin), strings.Index(s, tomlEnd); i >= 0 && j > i {
		after = []byte(strings.TrimRight(s[:i], "\n") + "\n" + strings.TrimLeft(s[j+len(tomlEnd):], "\n"))
		if strings.TrimSpace(string(after)) == "" {
			return []Change{{Path: p, Rel: "~/.codex/config.toml", What: "remove: aitk MCP server", before: before, Delete: true}}, nil
		}
	}
	return []Change{{Path: p, Rel: "~/.codex/config.toml", What: "remove: aitk MCP server", before: before, after: after}}, nil
}

// globalCmds installs (or removes) the built-in slash commands in a tool's user-level folder.
func globalCmds(e Env, dir, format string, remove bool) []Change {
	return commandChanges(e.Home, dir, "~/"+dir, slash.Formats[format], slash.Builtins(), true, remove)
}

// claudeUserMCP registers the server for every Claude Code project with Claude's own CLI (the
// user config file is rewritten by running sessions, so aitk does not edit it directly).
func claudeUserMCP(e Env, remove bool) []Change {
	claude, err := e.LookPath("claude")
	if err != nil {
		return nil
	}
	var cfg struct {
		MCPServers map[string]any `json:"mcpServers"`
	}
	cfgPath := filepath.Join(e.Home, ".claude.json")
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		cfgPath = filepath.Join(dir, ".claude.json")
	}
	json.Unmarshal(read(cfgPath), &cfg)
	_, present := cfg.MCPServers["aitk"]
	switch {
	case !remove && !present:
		return []Change{{Rel: "Claude Code user config", What: "register aitk MCP server (all projects)", Run: []string{claude, "mcp", "add", "--scope", "user", "aitk", "--", "aitk", "mcp"}}}
	case remove && present:
		return []Change{{Rel: "Claude Code user config", What: "remove: aitk MCP server", Run: []string{claude, "mcp", "remove", "--scope", "user", "aitk"}}}
	}
	return nil
}

// cliMCP registers the server through a tool's own CLI, when its listing does not show it yet.
func cliMCP(e Env, bin string, remove bool, list, add, del []string) []Change {
	path, err := e.LookPath(bin)
	if err != nil {
		return nil
	}
	list, add, del = withBin(path, list), withBin(path, add), withBin(path, del)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, list[0], list[1:]...).Output()
	if err != nil {
		return nil // cannot tell; leave it alone
	}
	present := false
	for _, l := range strings.Split(string(out), "\n") {
		if f := strings.Fields(l); len(f) > 0 && strings.TrimSuffix(f[0], ":") == "aitk" {
			present = true
		}
	}
	switch {
	case !remove && !present:
		return []Change{{Rel: bin + " MCP config", What: "register aitk MCP server", Run: add}}
	case remove && present:
		return []Change{{Rel: bin + " MCP config", What: "remove: aitk MCP server", Run: del}}
	}
	return nil
}

func withBin(path string, argv []string) []string {
	return append([]string{path}, argv[1:]...)
}
