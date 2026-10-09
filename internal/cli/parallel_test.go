package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParallelWorktreesMergeWithoutConflicts(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	var ids []string
	for _, title := range []string{"Feature A", "Feature B", "Feature C"} {
		r := aitk(t, dir, "task", "new", title, "--ac", title+" works", "--tag", strings.ToLower(strings.Fields(title)[1]))
		mustOK(t, r)
		ids = append(ids, data(r)["id"].(string))
	}
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "chore: plan three features")

	var paths []string
	for _, id := range ids {
		r := aitk(t, dir, "work", id)
		mustOK(t, r)
		paths = append(paths, data(r)["path"].(string))
	}
	// The main worktree cannot take a task another worktree has claimed.
	expect(t, aitk(t, dir, "task", "start", ids[0]), 4, "E_TASK_CLAIMED")
	expect(t, aitk(t, dir, "work", ids[1]), 4, "E_TASK_CLAIMED")

	for i, wt := range paths {
		name := []string{"a", "b", "c"}[i]
		write(t, wt, name+".txt", name)
		write(t, wt, "ok.txt", "1")
		mustOK(t, aitk(t, wt, "check"))
		mustOK(t, aitk(t, wt, "task", "update", "--ac-done", "1"))
		r := aitk(t, wt, "close", "--summary", "Implemented "+name, "--knowledge", "Feature "+name+" lives in "+name+".txt")
		mustOK(t, r)
		if data(r)["status"] != "done" {
			t.Fatalf("worktree %d close: %v", i, data(r))
		}
		run(t, wt, "git", "add", "-A")
		run(t, wt, "git", "commit", "-qm", "feat: "+name)
	}
	for _, id := range ids {
		branch := strings.TrimSpace(run(t, dir, "git", "for-each-ref", "--format=%(refname:short)", "refs/heads/aitk/"+id+"-*"))
		out := run(t, dir, "git", "merge", "--no-edit", "-q", branch)
		if strings.Contains(out, "CONFLICT") {
			t.Fatalf("merge conflict merging %s:\n%s", branch, out)
		}
	}
	if st := run(t, dir, "git", "status", "--porcelain"); strings.Contains(st, "UU ") || strings.Contains(st, "AA ") {
		t.Fatalf("unmerged paths:\n%s", st)
	}
	mustOK(t, aitk(t, dir, "doctor", "--fix"))
	list := aitk(t, dir, "task", "list", "--status", "done")
	if n := len(list.env["data"].([]any)); n != 3 {
		t.Fatalf("expected 3 done tasks after merging, got %d", n)
	}
	inbox := read(t, dir, "docs/ai/knowledge/_inbox.md")
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		if !strings.Contains(inbox+readAll(t, dir, "docs/ai/knowledge"), n) {
			t.Fatalf("knowledge from branch %s lost in merge", n)
		}
	}
	rep := aitk(t, dir, "report", "--since", "1d")
	mustOK(t, rep)
	if done := data(rep)["done"].([]any); len(done) != 3 || data(rep)["handoffs_by_tool"].(map[string]any)["claude-code"] == nil {
		t.Fatalf("report wrong: %v", data(rep))
	}
}

func readAll(t *testing.T, dir, rel string) string {
	t.Helper()
	var b strings.Builder
	entries, _ := os.ReadDir(filepath.Join(dir, rel))
	for _, e := range entries {
		if !e.IsDir() {
			x, _ := os.ReadFile(filepath.Join(dir, rel, e.Name()))
			b.Write(x)
		}
	}
	return b.String()
}

func TestSwitchAndLog(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	newStarted(t, dir, "Half done", "--ac", "first part", "--ac", "second part")
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	r := aitk(t, dir, "switch", "codex", "--note", "Second part needs the API client")
	mustOK(t, r)
	d := data(r)
	cmd, _ := d["command"].([]any)
	if d["tool"] != "codex" || len(cmd) != 2 || cmd[0] != "codex" {
		t.Fatalf("switch result: %v", d)
	}
	prompt, _ := os.ReadFile(d["prompt_file"].(string))
	if !strings.Contains(string(prompt), "# Brief:") || !strings.Contains(string(prompt), "second part") {
		t.Fatalf("prompt lacks the brief:\n%s", prompt)
	}
	h := read(t, dir, d["handoff"].(string))
	if !strings.Contains(h, "outcome: switched") || !strings.Contains(h, "to: codex") || !strings.Contains(h, "Satisfy criterion 2") {
		t.Fatalf("handoff:\n%s", h)
	}
	unknown := aitk(t, dir, "switch", "my-agent")
	mustOK(t, unknown)
	if data(unknown)["command"] != nil {
		t.Fatal("unknown tool must not get a guessed command")
	}
	lg := aitk(t, dir, "log", "--limit", "5")
	mustOK(t, lg)
	if entries := lg.env["data"].([]any); len(entries) != 2 {
		t.Fatalf("log entries: %d", len(entries))
	}
	expect(t, aitk(t, dir, "report", "--since", "yesterday"), 2, "E_USAGE")
}

func TestClaimExpiryAndRelease(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	r := aitk(t, dir, "task", "new", "Shared")
	id := data(r)["id"].(string)
	mustOK(t, aitk(t, dir, "task", "claim", id, "--ttl", "1h"))
	mustOK(t, aitk(t, dir, "task", "claim", id)) // renewing your own claim is fine
	rel := aitk(t, dir, "task", "release", id)
	if data(rel)["released"] != true {
		t.Fatal("release failed")
	}
}

func TestImportChecklistsAndDeploy(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "specs/001-checkout/tasks.md", "# Tasks\n- [ ] T001 [P] Create cart model\n- [x] T002 Add totals\n- [ ] Write docs\n")
	r := aitk(t, dir, "import", "spec-kit", "--dry-run")
	mustOK(t, r)
	if n := len(data(r)["imported"].([]any)); n != 3 {
		t.Fatalf("dry run should list 3, got %d", n)
	}
	mustOK(t, aitk(t, dir, "import", "spec-kit"))
	list := aitk(t, dir, "task", "list", "--all")
	ids := map[string]string{}
	for _, x := range list.env["data"].([]any) {
		m := x.(map[string]any)
		ids[m["id"].(string)] = m["status"].(string) + " " + m["title"].(string)
	}
	if ids["CHECKOUT-T001"] != "todo Create cart model" || ids["CHECKOUT-T002"] != "done Add totals" {
		t.Fatalf("imported tasks: %v", ids)
	}
	again := aitk(t, dir, "import", "spec-kit")
	if len(data(again)["skipped"].([]any)) != 3 {
		t.Fatal("re-import must skip existing tasks")
	}
	expect(t, aitk(t, dir, "deploy"), 2, "E_USAGE")
	cfg := read(t, dir, "aitk.toml")
	write(t, dir, "aitk.toml", cfg+"\n[deploy]\npost_check = \"git --version\"\n\n[deploy.targets]\nstaging = \"git status\"\nbroken = \"git definitely-not-a-command\"\n")
	mustOK(t, aitk(t, dir, "deploy", "staging"))
	expect(t, aitk(t, dir, "deploy", "broken"), 1, "E_DEPLOY_FAILED")
}
