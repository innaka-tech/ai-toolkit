package cli

import (
	"strings"
	"testing"
)

// asPerson clears every AI-agent marker so the next commands run as a person.
func asPerson(t *testing.T) {
	t.Helper()
	for _, v := range []string{"CLAUDECODE", "CODEX_SESSION_ID", "CODEX_SANDBOX", "CODEX_VERSION", "GEMINI_CLI", "OPENCODE", "CURSOR_AGENT"} {
		t.Setenv(v, "")
	}
	t.Setenv("AITK_TOOL", "human")
}

func fillRisk(t *testing.T, dir string) string {
	t.Helper()
	f := strings.TrimSpace(run(t, dir, "sh", "-c", "ls docs/ai/tasks/*.md"))
	body := "Impact: new table.\nSecurity (STRIDE): no new input.\nRollback: drop table.\nValidation: migration test."
	write(t, dir, f, strings.Replace(read(t, dir, f), "(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)", body, 1))
	return f
}

func TestStrictSecurityAuditReviewAndUAT(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), "sensitive_paths = []", `sensitive_paths = ["db/**"]`, 1)+"\n[uat]\nrequired = \"strict\"\n")
	id := newStarted(t, dir, "Add payments table", "--ac", "migration applies")
	write(t, dir, "db/001.sql", "create table payments(id int);\n")
	write(t, dir, "ok.txt", "1")
	f := fillRisk(t, dir)
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	close := func() res { return aitk(t, dir, "close", "--summary", "Added payments table", "--knowledge", "none") }

	expect(t, close(), 3, "E_DOD_SECURITY") // no checklist
	mustOK(t, aitk(t, dir, "security", "checklist"))
	expect(t, close(), 3, "E_DOD_SECURITY") // items unverified
	body := read(t, dir, f)
	write(t, dir, f, strings.ReplaceAll(body, "- [ ] V", "- [x] V"))
	expect(t, close(), 3, "E_DOD_AUDIT")
	mustOK(t, aitk(t, dir, "audit"))

	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	t.Setenv("AITK_TOOL", "codex")
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	t.Setenv("AITK_TOOL", "claude-code")
	r := close()
	mustOK(t, r)
	if data(r)["status"] != "in_review" || !strings.Contains(strings.Join(toStrings(r.env["warnings"]), " "), "user acceptance") {
		t.Fatalf("expected in_review waiting for UAT: %v %v", data(r), r.env["warnings"])
	}
	mustOK(t, aitk(t, dir, "uat", "script", id))
	if !strings.Contains(read(t, dir, "docs/ai/uat/"+id+".md"), "## Scenario 1") {
		t.Fatal("UAT script missing scenarios")
	}
	expect(t, aitk(t, dir, "uat", "accept", id), 3, "E_UAT_AGENT") // an agent cannot accept
	asPerson(t)
	acc := aitk(t, dir, "uat", "accept", id, "--by", "Anas", "--note", "paid an invoice in staging")
	mustOK(t, acc)
	if data(acc)["status"] != "done" {
		t.Fatalf("acceptance should complete the task: %v %v", data(acc), acc.env["warnings"])
	}
	if !strings.Contains(read(t, dir, f), "by: Anas") {
		t.Fatal("acceptance not recorded on the task")
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestUATRejectReturnsTaskToWork(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "aitk.toml", read(t, dir, "aitk.toml")+"\n[uat]\nrequired = \"all\"\n")
	id := newStarted(t, dir, "Export CSV", "--ac", "csv downloads")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	r := aitk(t, dir, "close", "--summary", "Export added", "--knowledge", "none")
	if data(r)["status"] != "in_review" {
		t.Fatalf("all-profile UAT should hold the task: %v", data(r))
	}
	asPerson(t)
	expect(t, aitk(t, dir, "uat", "reject", id), 2, "E_USAGE")
	rej := aitk(t, dir, "uat", "reject", id, "--reason", "dates are in UTC, expected WIB")
	mustOK(t, rej)
	if data(rej)["status"] != "in_progress" {
		t.Fatalf("reject should reopen: %v", data(rej))
	}
	if !strings.Contains(data(aitk(t, dir, "brief"))["markdown"].(string), "dates are in UTC, expected WIB") {
		t.Fatal("brief should carry the UAT feedback to the next agent")
	}
}

func TestAuditFindsCommittedSecret(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	newStarted(t, dir, "Config", "--ac", "loads")
	mustOK(t, aitk(t, dir, "audit"))
	token := "gh" + "p_" + "aB3dE5fG7hI9jK1lM3nO5pQ7rS9tU1vW3xY5"
	write(t, dir, "config.env", "TOKEN="+token+"\n")
	run(t, dir, "git", "add", "config.env")
	run(t, dir, "git", "commit", "-qm", "chore: config", "--no-verify")
	r := aitk(t, dir, "audit")
	expect(t, r, 1, "E_AUDIT_FAILED")
	if strings.Contains(r.stdout, token) || !strings.Contains(r.stdout, "config.env") {
		t.Fatalf("audit must name the file without printing the secret:\n%s", r.stdout)
	}
}
