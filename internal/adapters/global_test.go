package adapters

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSetupRoundTrip(t *testing.T) {
	e := env(t, "claude", "codex", "opencode")
	put(t, filepath.Join(e.Home, ".claude/CLAUDE.md"), "# mine\n\nBe terse.\n")
	put(t, filepath.Join(e.Home, ".config/opencode/opencode.json"), "{\n  \"provider\": {\"x\": 1}\n}\n")
	put(t, filepath.Join(e.Home, ".codex/config.toml"), "model = \"o3\"\n")
	before := map[string]string{
		".claude/CLAUDE.md":              get(t, filepath.Join(e.Home, ".claude/CLAUDE.md")),
		".config/opencode/opencode.json": get(t, filepath.Join(e.Home, ".config/opencode/opencode.json")),
		".codex/config.toml":             get(t, filepath.Join(e.Home, ".codex/config.toml")),
	}
	r, err := Setup(e, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range r.Changes {
		if c.Tool == "gemini-cli" || c.Tool == "kiro" {
			t.Fatalf("tools that are not installed must be left alone: %+v", c)
		}
	}
	if err := Apply(r.Changes, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	claude := get(t, filepath.Join(e.Home, ".claude/CLAUDE.md"))
	if !strings.HasPrefix(claude, "# mine\n\nBe terse.\n") || !strings.Contains(claude, "aitk.toml") {
		t.Fatalf("user text must stay and the block must be added:\n%s", claude)
	}
	if get(t, filepath.Join(e.Home, ".claude/skills/aitk/SKILL.md")) == "" {
		t.Fatal("user-level skill missing")
	}
	oc := get(t, filepath.Join(e.Home, ".config/opencode/opencode.json"))
	if !strings.Contains(oc, `"x": 1`) || !strings.Contains(oc, `"aitk"`) {
		t.Fatalf("opencode config must keep the user's settings and gain the MCP server:\n%s", oc)
	}
	if !strings.Contains(get(t, filepath.Join(e.Home, ".codex/AGENTS.md")), "aitk brief") {
		t.Fatal("codex user instructions missing")
	}
	if r, _ := Setup(e, nil, false); len(r.Changes) != 0 {
		t.Fatalf("setup must be idempotent: %+v", r.Changes)
	}
	r, _ = Setup(e, nil, true)
	if err := Apply(r.Changes, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for rel, want := range before {
		got := get(t, filepath.Join(e.Home, rel))
		if strings.TrimSpace(got) != strings.TrimSpace(want) && !(strings.HasSuffix(rel, ".json") && !strings.Contains(got, "aitk")) {
			t.Fatalf("%s not restored:\n%s\nwant:\n%s", rel, got, want)
		}
	}
	for _, rel := range []string{".codex/AGENTS.md", ".config/opencode/AGENTS.md", ".claude/skills/aitk/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(e.Home, rel)); err == nil {
			t.Fatalf("%s was created by setup and must be removed", rel)
		}
	}
}

func TestSetupLeavesHandWrittenSkillAlone(t *testing.T) {
	e := env(t, "claude")
	put(t, filepath.Join(e.Home, ".claude/skills/aitk/SKILL.md"), "my own skill\n")
	r, _ := Setup(e, nil, true)
	for _, c := range r.Changes {
		if c.Delete && strings.Contains(c.Rel, "skills") {
			t.Fatal("a skill that is not aitk's must not be deleted")
		}
	}
}

func TestMoreToolsAndAll(t *testing.T) {
	e := env(t)
	put(t, filepath.Join(e.Root, ".clinerules"), "Existing cline rules.\n")
	put(t, filepath.Join(e.Root, ".aider.conf.yml"), "model: sonnet\n")
	cs, err := Plan(e, []string{"all"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(cs, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		".github/copilot-instructions.md": "AGENTS.md",
		".vscode/mcp.json":                `"servers"`,
		".windsurf/rules/aitk.md":         "trigger: always_on",
		".clinerules":                     "Existing cline rules.",
		".roo/rules/aitk.md":              "aitk brief",
		".roo/mcp.json":                   `"aitk"`,
		".junie/guidelines.md":            "aitk brief",
		".qwen/settings.json":             "AGENTS.md",
		".aider.conf.yml":                 "read: [AGENTS.md]",
	} {
		if got := get(t, filepath.Join(e.Root, rel)); !strings.Contains(got, want) {
			t.Errorf("%s lacks %q:\n%s", rel, want, got)
		}
	}
	if !strings.Contains(get(t, filepath.Join(e.Root, ".clinerules")), "aitk brief") {
		t.Error("a single .clinerules file gets the aitk block")
	}
	if cs, _ := Plan(e, []string{"all"}, false); len(cs) != 0 {
		t.Fatalf("second sync must change nothing: %+v", cs)
	}
}

// Review: setup never loses a user's text, whatever state the file is in.
func TestSetupRefusesUnsafeFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions and symlinks")
	}
	e := env(t, "claude", "codex", "opencode")
	claude := filepath.Join(e.Home, ".claude/CLAUDE.md")
	put(t, claude, "MY PRECIOUS RULES\n")
	os.Chmod(claude, 0o000)
	defer os.Chmod(claude, 0o644)
	codex := filepath.Join(e.Home, ".codex/AGENTS.md")
	put(t, codex, "top\n"+GlobalBegin+"\nUSER CONTENT\n") // a stray begin marker
	oc := filepath.Join(e.Home, ".config/opencode/AGENTS.md")
	put(t, oc, "mentions "+GlobalEnd+" in prose\n"+GlobalBegin+"\n")
	r, err := Setup(e, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(r.Changes, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	os.Chmod(claude, 0o644)
	if got := get(t, claude); got != "MY PRECIOUS RULES\n" {
		t.Fatalf("an unreadable file must be left alone:\n%s", got)
	}
	if got := get(t, codex); got != "top\n"+GlobalBegin+"\nUSER CONTENT\n" {
		t.Fatalf("unbalanced markers must be left alone:\n%s", got)
	}
	if strings.Count(get(t, oc), GlobalBegin) != 1 {
		t.Fatal("a file with markers out of order must not grow")
	}
	if len(r.Skipped) < 3 {
		t.Fatalf("each refusal must be reported: %v", r.Skipped)
	}
}

func TestSetupReadOnlyAndHandWrittenFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	e := env(t, "claude", "kiro")
	ro := filepath.Join(e.Home, ".claude/CLAUDE.md")
	put(t, ro, "read only\n")
	os.Chmod(ro, 0o444)
	defer os.Chmod(ro, 0o644)
	put(t, filepath.Join(e.Home, ".kiro/steering/aitk.md"), "my own steering\n")
	r, _ := Setup(e, nil, false)
	Apply(r.Changes, t.TempDir())
	if get(t, ro) != "read only\n" || get(t, filepath.Join(e.Home, ".kiro/steering/aitk.md")) != "my own steering\n" {
		t.Fatal("read-only and hand-written files must be left alone")
	}
}

// Review: --remove through a symlinked dotfile keeps the link.
func TestSetupRemoveKeepsSymlinkedDotfiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	e := env(t, "codex")
	target := filepath.Join(e.Home, "dotfiles/codex-agents.md")
	put(t, target, "")
	os.MkdirAll(filepath.Join(e.Home, ".codex"), 0o755)
	link := filepath.Join(e.Home, ".codex/AGENTS.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	r, _ := Setup(e, []string{"codex"}, false)
	Apply(r.Changes, t.TempDir())
	if !strings.Contains(get(t, target), "aitk") {
		t.Fatal("setup must write through the link")
	}
	r, _ = Setup(e, []string{"codex"}, true)
	Apply(r.Changes, t.TempDir())
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("remove must keep the symlink")
	}
	if strings.Contains(get(t, target), "aitk") {
		t.Fatal("remove must take the block out of the link's target")
	}
}

// Review: one tool's JSONC config does not stop the others; Copilot needs the extension.
func TestSyncKeepsGoingAndCopilotDetection(t *testing.T) {
	e := env(t, "code")
	put(t, filepath.Join(e.Root, ".vscode/settings.json"), "{}\n")
	for _, a := range all() {
		if a.name == "copilot" && a.installed(e) {
			t.Fatal("VS Code alone is not Copilot")
		}
	}
	put(t, filepath.Join(e.Root, ".vscode/mcp.json"), "{\n  // comment\n  \"servers\": {}\n}\n")
	cs, skipped, err := PlanSkipping(e, []string{"all"}, false)
	if err != nil || len(cs) == 0 || len(skipped) == 0 {
		t.Fatalf("other tools must still be planned and the JSONC file reported: %d changes, %v, %v", len(cs), skipped, err)
	}
}
