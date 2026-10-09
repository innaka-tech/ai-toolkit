package plugins_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/innaka-tech/ai-toolkit/internal/cli"
)

func setup(t *testing.T, enable bool, timeout string) string {
	t.Helper()
	bindir := t.TempDir()
	name := "aitk-demo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bindir, name), "./testdata/aitk-demo").CombinedOutput(); err != nil {
		t.Fatalf("build plugin: %v %s", err, out)
	}
	t.Setenv("PATH", bindir+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@e.st"}, {"config", "user.name", "t"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Run()
	}
	run(t, dir, "init")
	cfg, _ := os.ReadFile(filepath.Join(dir, "aitk.toml"))
	extra := "\n[plugins]\ntimeout = \"" + timeout + "\"\n"
	if enable {
		extra += "enabled = [\"demo\"]\n\n[plugins.settings.demo]\nnamespace = \"shop\"\n"
	}
	os.WriteFile(filepath.Join(dir, "aitk.toml"), append(cfg, []byte(extra)...), 0o644)
	return dir
}

func run(t *testing.T, dir string, args ...string) map[string]any {
	t.Helper()
	var out, errb bytes.Buffer
	cli.Execute(append([]string{"--json", "-C", dir}, args...), &out, &errb)
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("%v: %s", args, out.String())
	}
	if env["ok"] != true {
		t.Fatalf("%v failed: %s", args, out.String())
	}
	return env
}

func TestEnabledPluginContributes(t *testing.T) {
	dir := setup(t, true, "5s")
	b := run(t, dir, "brief")["data"].(map[string]any)
	if md := b["markdown"].(string); !strings.Contains(md, "Demo memory (plugin demo)") || !strings.Contains(md, "namespace=shop") {
		t.Fatalf("plugin section or settings missing:\n%s", md)
	}
	doc := run(t, dir, "doctor")["data"].(map[string]any)
	found := false
	for _, c := range doc["checks"].([]any) {
		found = found || strings.Contains(c.(map[string]any)["name"].(string), "plugin demo: demo backend")
	}
	if !found {
		t.Fatal("doctor did not include plugin check")
	}
	hits := run(t, dir, "knowledge", "search", "deploy")["data"].([]any)
	if len(hits) == 0 || !strings.Contains(hits[0].(map[string]any)["text"].(string), "Fridays") {
		t.Fatalf("plugin search result missing: %v", hits)
	}
	list := run(t, dir, "plugin", "list")["data"].([]any)
	if len(list) == 0 || list[0].(map[string]any)["enabled"] != true || len(list[0].(map[string]any)["hooks"].([]any)) != 4 {
		t.Fatalf("plugin list: %v", list)
	}
}

func TestDisabledPluginIsNotCalled(t *testing.T) {
	dir := setup(t, false, "5s")
	md := run(t, dir, "brief")["data"].(map[string]any)["markdown"].(string)
	if strings.Contains(md, "plugin demo") {
		t.Fatal("disabled plugin was called")
	}
}

func TestSlowPluginTimesOutWithoutFailingTheCommand(t *testing.T) {
	dir := setup(t, true, "500ms")
	t.Setenv("AITK_DEMO_SLOW", "1")
	env := run(t, dir, "brief")
	ws, _ := env["warnings"].([]any)
	data := env["data"].(map[string]any)
	errs, _ := data["plugin_errors"].([]any)
	if len(errs) == 0 || !strings.Contains(errs[0].(string), "timed out") {
		t.Fatalf("expected a timeout plugin error, got %v / %v", errs, ws)
	}
}
