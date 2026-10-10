package slash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinsAreComplete(t *testing.T) {
	want := []string{"aitk-check", "aitk-close", "aitk-fix", "aitk-handover", "aitk-hunt", "aitk-impact", "aitk-next", "aitk-plan", "aitk-release", "aitk-review", "aitk-start", "aitk-status", "aitk-uat"}
	got := map[string]Command{}
	for _, c := range Builtins() {
		got[c.Name] = c
	}
	for _, n := range want {
		c, ok := got[n]
		if !ok || c.Description == "" || len(c.Body) < 80 {
			t.Errorf("built-in %s missing or empty: %+v", n, c)
		}
		if strings.Contains(c.Body, "${") {
			t.Errorf("%s uses a placeholder no tool understands", n)
		}
	}
	// The reviewer must not become a worker of the task it reviews.
	if r := got["aitk-review"]; !strings.Contains(r.Body, "Do NOT run `aitk task start`") {
		t.Error("the review persona must forbid starting the task")
	}
}

func TestProjectCommandsOverrideAndValidate(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ProjectDir)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "deploy-check.md"), []byte("---\ndescription: Check a deploy\nargument-hint: \"<version>\"\n---\nCheck $ARGUMENTS.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "aitk-review.md"), []byte("Our own review steps.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "Bad Name.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("docs"), 0o644)
	all, warns := All(root)
	byName := map[string]Command{}
	for _, c := range all {
		byName[c.Name] = c
	}
	if c := byName["deploy-check"]; c.Description != "Check a deploy" || c.ArgHint != "<version>" || c.Expand("v1.2") != "Check v1.2.\n" {
		t.Fatalf("project command parsed wrong: %+v", c)
	}
	if byName["aitk-review"].Source == "builtin" || len(warns) != 1 {
		t.Fatalf("a project file replaces the built-in of that name; bad names are reported: %v", warns)
	}
	if _, ok := byName["README"]; ok {
		t.Fatal("README.md is documentation, not a command")
	}
}

func TestFormats(t *testing.T) {
	c := Command{Name: "x", Description: `Say "hi"`, ArgHint: "<who>", Body: "Greet $ARGUMENTS with ''' and \\ and \"\"\".\n", Source: "builtin"}
	for name, f := range Formats {
		out := string(f.Render(c))
		if !Generated([]byte(out)) {
			t.Errorf("%s: no generated marker", name)
		}
		switch name {
		case "gemini":
			if !strings.Contains(out, "{{args}}") || !strings.Contains(out, `description = "Say \"hi\""`) || !strings.Contains(out, `prompt = """`) {
				t.Errorf("gemini TOML wrong:\n%s", out)
			}
		case "copilot":
			if !strings.Contains(out, "${input:args}") || !strings.Contains(out, "mode: agent") {
				t.Errorf("copilot prompt file wrong:\n%s", out)
			}
		case "claude", "opencode":
			if !strings.Contains(out, "$ARGUMENTS") || !strings.HasPrefix(out, "---\n") {
				t.Errorf("%s markdown wrong:\n%s", name, out)
			}
		case "cursor", "windsurf":
			if strings.Contains(out, "$ARGUMENTS") {
				t.Errorf("%s has no placeholder support:\n%s", name, out)
			}
		}
	}
}
