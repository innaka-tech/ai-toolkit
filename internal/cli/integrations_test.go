package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func tasksByID(t *testing.T, dir string) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, x := range aitk(t, dir, "task", "list", "--all").env["data"].([]any) {
		m := x.(map[string]any)
		out[m["id"].(string)] = m
	}
	return out
}

func TestImportBMAD(t *testing.T) {
	src, _ := filepath.Abs("../../tests/fixtures/bmad")
	dir := repo(t)
	initRepo(t, dir)
	copyTree(t, src, dir)
	r := aitk(t, dir, "import", "bmad")
	mustOK(t, r)
	if n := len(data(r)["imported"].([]any)); n != 2 {
		t.Fatalf("imported %d tickets, want 2: %v", n, data(r))
	}
	ts := tasksByID(t, dir)
	story := ts["BM-CART-RULES-2"]
	if story == nil || story["status"] != "in_progress" || story["profile"] != "strict" || story["profile_pinned"] != true {
		t.Fatalf("discount story: %v (all: %v)", story, ts)
	}
	if done := ts["BM-CART-RULES-1"]; done == nil || done["status"] != "done" {
		t.Fatalf("scaffold story should be done from its plan: %v", done)
	}
	show := aitk(t, dir, "task", "show", "BM-CART-RULES-2")
	cs := data(show)["criteria"].([]any)
	if len(cs) != 2 || !strings.Contains(cs[0].(map[string]any)["text"].(string), "Valid code reduces the total: Given a cart of 200000") {
		t.Fatalf("criteria: %v", cs)
	}
	verify := data(aitk(t, dir, "task", "show", "BM-CART-RULES-1"))["criteria"].([]any)
	if len(verify) != 1 || !strings.HasPrefix(verify[0].(map[string]any)["text"].(string), "Verify: GET /health") {
		t.Fatalf("verify criterion: %v", verify)
	}
	if again := aitk(t, dir, "import", "bmad"); len(data(again)["skipped"].([]any)) != 2 {
		t.Fatal("re-import must skip")
	}
}

func TestImportSuperpowers(t *testing.T) {
	src, _ := filepath.Abs("../../tests/fixtures/superpowers")
	dir := repo(t)
	initRepo(t, dir)
	copyTree(t, src, dir)
	mustOK(t, aitk(t, dir, "import", "superpowers"))
	ts := tasksByID(t, dir)
	one, two := ts["SP-INVOICE-EXPORT-1"], ts["SP-INVOICE-EXPORT-2"]
	if one == nil || one["status"] != "done" || one["title"] != "CSV writer" {
		t.Fatalf("task 1: %v (all %v)", one, ts)
	}
	if two == nil || two["status"] != "todo" {
		t.Fatalf("task 2: %v", two)
	}
	sh := aitk(t, dir, "task", "show", "SP-INVOICE-EXPORT-2")
	if cs := data(sh)["criteria"].([]any); len(cs) != 2 || !strings.Contains(cs[1].(map[string]any)["text"].(string), "Step 2: Implement the route") {
		t.Fatalf("steps as criteria: %v", cs)
	}
	if !strings.Contains(read(t, dir, data(sh)["task"].(map[string]any)["file"].(string)), "Modify: `src/routes/invoices.ts`") {
		t.Fatal("files list missing from objective")
	}
}

func TestImportMarkdown(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "notes/backlog.md", "# Backlog\n- [ ] Add dark mode\n- [x] Fix login typo\n")
	expect(t, aitk(t, dir, "import", "markdown"), 2, "E_USAGE")
	mustOK(t, aitk(t, dir, "import", "markdown", "notes/backlog.md"))
	if n := len(tasksByID(t, dir)); n != 2 {
		t.Fatalf("markdown import: %d tasks", n)
	}
}
