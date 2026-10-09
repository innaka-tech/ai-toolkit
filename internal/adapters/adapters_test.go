package adapters

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func env(t *testing.T, bins ...string) Env {
	t.Helper()
	root, home := t.TempDir(), t.TempDir()
	return Env{Root: root, Home: home, LookPath: func(b string) (string, error) {
		for _, x := range bins {
			if x == b {
				return "/usr/bin/" + b, nil
			}
		}
		return "", errors.New("not found")
	}}
}

func put(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func sync(t *testing.T, e Env, global bool, tools ...string) []Change {
	t.Helper()
	cs, err := Plan(e, tools, global)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(cs, filepath.Join(e.Home, "backups")); err != nil {
		t.Fatal(err)
	}
	return cs
}

func TestDetectsOnlyInstalledTools(t *testing.T) {
	e := env(t, "claude", "opencode")
	cs := sync(t, e, false)
	tools := map[string]bool{}
	for _, c := range cs {
		tools[c.Tool] = true
	}
	if !tools["claude-code"] || !tools["opencode"] || tools["gemini-cli"] || tools["codex"] {
		t.Fatalf("wrong tools: %v", tools)
	}
	if _, err := Plan(e, []string{"vim"}, false); err == nil {
		t.Fatal("unknown tool accepted")
	}
}

func TestClaudePreservesUserSettingsAndIsIdempotent(t *testing.T) {
	e := env(t, "claude")
	user := `{
  "permissions": {"allow": ["Bash(make test)"]},
  "zeta": 1,
  "hooks": {"SessionStart": [{"matcher": "startup", "hooks": [{"type": "command", "command": "echo mine"}]}]},
  "alpha": true
}`
	put(t, filepath.Join(e.Root, ".claude/settings.json"), user)
	put(t, filepath.Join(e.Root, ".mcp.json"), `{"mcpServers": {"db": {"command": "dbmcp"}}}`)
	put(t, filepath.Join(e.Root, "CLAUDE.md"), "# Team notes\n")
	sync(t, e, false, "claude-code")

	s := get(t, filepath.Join(e.Root, ".claude/settings.json"))
	if strings.Index(s, `"permissions"`) > strings.Index(s, `"zeta"`) || strings.Index(s, `"zeta"`) > strings.Index(s, `"alpha"`) {
		t.Fatalf("key order changed:\n%s", s)
	}
	if !strings.Contains(s, "echo mine") || strings.Count(s, "aitk brief") != 1 {
		t.Fatalf("user hook lost or aitk hook missing:\n%s", s)
	}
	var mcp struct{ MCPServers map[string]map[string]any }
	json.Unmarshal([]byte(get(t, filepath.Join(e.Root, ".mcp.json"))), &mcp)
	if mcp.MCPServers["db"] == nil || mcp.MCPServers["aitk"]["command"] != "aitk" {
		t.Fatalf("mcp servers wrong: %v", mcp.MCPServers)
	}
	if !strings.Contains(get(t, filepath.Join(e.Root, ".claude/skills/aitk/SKILL.md")), "name: aitk") {
		t.Fatal("Agent Skill not installed")
	}
	if got := get(t, filepath.Join(e.Root, "CLAUDE.md")); got != "@AGENTS.md\n\n# Team notes\n" {
		t.Fatalf("CLAUDE.md: %q", got)
	}
	if again := sync(t, e, false, "claude-code"); len(again) != 0 {
		t.Fatalf("second sync not a no-op: %v", again)
	}
}

func TestGeminiOpencodeKiroCursor(t *testing.T) {
	e := env(t, "gemini", "opencode", "kiro-cli", "cursor")
	put(t, filepath.Join(e.Root, ".gemini/settings.json"), `{"context": {"fileName": "GEMINI.md"}, "theme": "dark"}`)
	sync(t, e, false)
	var g struct {
		Context    struct{ FileName []string }
		MCPServers map[string]any
		Hooks      map[string]any
		Theme      string
	}
	json.Unmarshal([]byte(get(t, filepath.Join(e.Root, ".gemini/settings.json"))), &g)
	if g.Theme != "dark" || len(g.Context.FileName) != 2 || g.Context.FileName[0] != "AGENTS.md" || g.MCPServers["aitk"] == nil || g.Hooks["SessionStart"] == nil {
		t.Fatalf("gemini settings wrong: %+v", g)
	}
	oc := get(t, filepath.Join(e.Root, "opencode.json"))
	if !strings.Contains(oc, `"$schema"`) || !strings.Contains(oc, `"type": "local"`) {
		t.Fatalf("opencode.json: %s", oc)
	}
	if !strings.Contains(get(t, filepath.Join(e.Root, ".kiro/steering/aitk.md")), "inclusion: always") {
		t.Fatal("kiro steering missing")
	}
	if !strings.Contains(get(t, filepath.Join(e.Root, ".cursor/rules/aitk.mdc")), "alwaysApply: true") {
		t.Fatal("cursor rule missing")
	}
	if again := sync(t, e, false); len(again) != 0 {
		t.Fatalf("second sync not a no-op: %v", again)
	}
}

func TestCodexGlobalWithMarkersAndBackup(t *testing.T) {
	e := env(t, "codex")
	cfg := filepath.Join(e.Home, ".codex/config.toml")
	put(t, cfg, "model = \"o3\"\n\n[mcp_servers.db]\ncommand = \"dbmcp\"\n")
	if cs := sync(t, e, false, "codex"); len(cs) != 0 {
		t.Fatal("codex project sync must not touch global config")
	}
	sync(t, e, true, "codex")
	s := get(t, cfg)
	if !strings.HasPrefix(s, "model = \"o3\"\n\n[mcp_servers.db]") || strings.Count(s, "[mcp_servers.aitk]") != 1 {
		t.Fatalf("codex config wrong:\n%s", s)
	}
	backups, _ := filepath.Glob(filepath.Join(e.Home, "backups", "*", ".codex", "config.toml"))
	if len(backups) != 1 || get(t, backups[0]) != "model = \"o3\"\n\n[mcp_servers.db]\ncommand = \"dbmcp\"\n" {
		t.Fatalf("backup missing or wrong: %v", backups)
	}
	if again := sync(t, e, true, "codex"); len(again) != 0 {
		t.Fatal("second global sync not a no-op")
	}
}

func TestRejectsJSONCInsteadOfCorrupting(t *testing.T) {
	e := env(t, "opencode")
	p := filepath.Join(e.Root, "opencode.json")
	put(t, p, "{\n  // my comment\n  \"model\": \"x\"\n}\n")
	if _, err := Plan(e, []string{"opencode"}, false); err == nil || !strings.Contains(err.Error(), "by hand") {
		t.Fatalf("expected a clear refusal, got %v", err)
	}
	if !strings.Contains(get(t, p), "// my comment") {
		t.Fatal("file was modified")
	}
}

func TestDoctor(t *testing.T) {
	e := env(t, "claude")
	st := Doctor(e, false)
	for _, s := range st {
		if s.Tool == "claude-code" && (s.Current || len(s.Pending) == 0) {
			t.Fatalf("claude should need sync: %+v", s)
		}
	}
	sync(t, e, false)
	for _, s := range Doctor(e, false) {
		if s.Tool == "claude-code" && !s.Current {
			t.Fatalf("claude should be current: %+v", s)
		}
	}
}

// Review #10: global edits follow symlinks and keep restrictive permissions.
func TestGlobalEditKeepsSymlinkAndPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	e := env(t, "codex")
	real := filepath.Join(e.Home, "dotfiles", "codex.toml")
	put(t, real, "model = \"o3\"\n")
	os.Chmod(real, 0o600)
	os.MkdirAll(filepath.Join(e.Home, ".codex"), 0o755)
	link := filepath.Join(e.Home, ".codex", "config.toml")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	sync(t, e, true, "codex")
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced by a regular file")
	}
	if fi, _ := os.Stat(real); fi.Mode().Perm() != 0o600 {
		t.Fatalf("permissions changed to %v", fi.Mode().Perm())
	}
	if !strings.Contains(get(t, real), "[mcp_servers.aitk]") {
		t.Fatal("target not updated")
	}
}
