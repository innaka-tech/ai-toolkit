package mcpserver_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/innaka-tech/ai-toolkit/v2/internal/cli"
	"github.com/innaka-tech/ai-toolkit/v2/internal/mcpserver"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@e.st"}, {"config", "user.name", "t"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	var out, errb bytes.Buffer
	if code := cli.Execute([]string{"-C", dir, "init"}, &out, &errb); code != 0 {
		t.Fatalf("init failed: %s %s", out.String(), errb.String())
	}
	check := "test -f ok.txt"
	if runtime.GOOS == "windows" {
		check = "if exist ok.txt (exit 0) else (exit 1)"
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "aitk.toml"))
	os.WriteFile(filepath.Join(dir, "aitk.toml"), append(cfg, []byte("\n[check]\ncmd = '"+check+"'\n")...), 0o644)
	return dir
}

func connect(t *testing.T, dir, clientName string) *mcp.ClientSession {
	t.Helper()
	exec := func(args []string, stdout, stderr *bytes.Buffer) int { return cli.Execute(args, stdout, stderr) }
	server := mcpserver.New(dir, "test", exec)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func callText(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func TestToolsAreListedWithSchemas(t *testing.T) {
	os.Unsetenv("AITK_TOOL")
	cs := connect(t, gitRepo(t), "claude-code")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"brief": false, "task_new": false, "task_start": false, "task_update": false, "task_list": false, "task_show": false,
		"check": false, "close": false, "review_pass": false, "knowledge_search": false, "knowledge_add": false, "adr_new": false, "doctor": false}
	for _, tool := range res.Tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
		}
		if tool.Description == "" || tool.InputSchema == nil {
			t.Errorf("tool %s lacks description or schema", tool.Name)
		}
	}
	for n, seen := range want {
		if !seen {
			t.Errorf("tool %s missing", n)
		}
	}
	if cs.InitializeResult().Instructions == "" {
		t.Error("server instructions missing")
	}
}

func TestFullFlowOverMCP(t *testing.T) {
	os.Unsetenv("AITK_TOOL")
	t.Cleanup(func() { os.Unsetenv("AITK_TOOL") })
	dir := gitRepo(t)
	cs := connect(t, dir, "claude-code")

	brief, isErr := callText(t, cs, "brief", nil)
	if isErr || !strings.Contains(brief, "# Brief:") {
		t.Fatalf("brief: %v %s", isErr, brief)
	}
	out, isErr := callText(t, cs, "task_new", map[string]any{"title": "Add ok file", "criteria": []string{"make check pass"}, "start": true})
	if isErr || !strings.Contains(out, `"created_by": "claude-code"`) {
		t.Fatalf("task_new should succeed and record the MCP client as tool: %v %s", isErr, out)
	}
	out, isErr = callText(t, cs, "close", map[string]any{"summary": "done", "knowledge": "none"})
	if !isErr || !strings.Contains(out, "E_DOD_CHECK_STALE") || !strings.Contains(out, "fix: aitk check") {
		t.Fatalf("close before check must be a tool error with a fix: %v %s", isErr, out)
	}
	os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("1"), 0o644)
	if out, isErr = callText(t, cs, "check", nil); isErr {
		t.Fatalf("check: %s", out)
	}
	if out, isErr = callText(t, cs, "task_update", map[string]any{"ac_done": []int{1}}); isErr {
		t.Fatalf("task_update: %s", out)
	}
	out, isErr = callText(t, cs, "close", map[string]any{"summary": "Added ok.txt", "knowledge": "The check requires ok.txt", "tags": []string{"build"}})
	if isErr || !strings.Contains(out, `"status": "done"`) {
		t.Fatalf("close: %v %s", isErr, out)
	}
	hits, _ := callText(t, cs, "knowledge_search", map[string]any{"query": "ok.txt check"})
	if !strings.Contains(hits, "requires ok.txt") {
		t.Fatalf("knowledge not searchable: %s", hits)
	}
	r, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "aitk://brief"})
	if err != nil || len(r.Contents) == 0 || !strings.Contains(r.Contents[0].Text, "# Brief:") {
		t.Fatalf("brief resource: %v", err)
	}
	p, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "close-session"})
	if err != nil || len(p.Messages) == 0 {
		t.Fatalf("prompt: %v", err)
	}
}

func TestToolName(t *testing.T) {
	for in, want := range map[string]string{"claude-code": "claude-code", "codex-mcp-client": "codex", "gemini-cli-mcp-client": "gemini-cli", "My Editor 2": "my-editor-2", "": "mcp-client"} {
		if got := mcpserver.ToolName(in); got != want {
			t.Errorf("ToolName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestStdioBinary runs the real binary as an MCP server over stdio, like an AI tool does.
func TestStdioBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "aitk")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "../../cmd/aitk")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	dir := gitRepo(t)
	cmd := exec.Command(bin, "mcp")
	cmd.Dir = dir
	client := mcp.NewClient(&mcp.Implementation{Name: "codex-mcp-client", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	out, isErr := callText(t, cs, "task_new", map[string]any{"title": "Over stdio"})
	if isErr || !strings.Contains(out, `"created_by": "codex"`) {
		t.Fatalf("stdio task_new: %v %s", isErr, out)
	}
}

// Review #13: free text that looks like a flag is passed as text, never parsed as a flag.
func TestFreeTextIsNeverParsedAsFlags(t *testing.T) {
	os.Unsetenv("AITK_TOOL")
	dir := gitRepo(t)
	other := gitRepo(t)
	cs := connect(t, dir, "claude-code")
	if out, isErr := callText(t, cs, "knowledge_add", map[string]any{"text": "-race flag must be used in CI"}); isErr {
		t.Fatalf("text starting with '-' rejected: %s", out)
	}
	if out, _ := callText(t, cs, "knowledge_search", map[string]any{"query": "-race"}); !strings.Contains(out, "-race flag must be used") {
		t.Fatalf("search with leading '-': %s", out)
	}
	out, isErr := callText(t, cs, "task_update", map[string]any{"id": "--path=" + other, "note": "x"})
	if !isErr || !strings.Contains(out, "E_TASK_NOT_FOUND") {
		t.Fatalf("an id that looks like a flag must be treated as an id: %v %s", isErr, out)
	}
	if out, isErr := callText(t, cs, "task_new", map[string]any{"title": "--help me please"}); isErr || !strings.Contains(out, "--help me please") {
		t.Fatalf("title starting with '--': %v %s", isErr, out)
	}
}

func TestSlashCommandsAreMCPPrompts(t *testing.T) {
	dir := gitRepo(t)
	os.MkdirAll(filepath.Join(dir, "docs/ai/commands"), 0o755)
	os.WriteFile(filepath.Join(dir, "docs/ai/commands/deploy-check.md"), []byte("---\ndescription: Check a deploy\n---\nCheck $ARGUMENTS.\n"), 0o644)
	cs := connect(t, dir, "claude-code")
	res, err := cs.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, p := range res.Prompts {
		names[p.Name] = true
	}
	for _, n := range []string{"aitk-review", "aitk-hunt", "aitk-plan", "deploy-check", "start-session", "close-session"} {
		if !names[n] {
			t.Errorf("prompt %s missing", n)
		}
	}
	got, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "deploy-check", Arguments: map[string]string{"args": "v1.2"}})
	if err != nil || got.Messages[0].Content.(*mcp.TextContent).Text != "Check v1.2.\n" {
		t.Fatalf("prompt arguments not filled in: %v %v", got, err)
	}
}
