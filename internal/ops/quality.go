package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/v2/internal/audit"
	"github.com/innaka-tech/ai-toolkit/v2/internal/claims"
	"github.com/innaka-tech/ai-toolkit/v2/internal/doc"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/handoff"
	"github.com/innaka-tech/ai-toolkit/v2/internal/profile"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/session"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
)

// ---------- OWASP ASVS ----------

// ASVSSection is the security checklist section name.
const ASVSSection = "Security checklist (OWASP ASVS 4.0.3)"

// asvsItems are the ASVS 4.0.3 chapters, phrased as level-1 verification questions.
var asvsItems = []string{
	"V1 Architecture: trust boundaries and the components handling sensitive data are identified for this change",
	"V2 Authentication: credentials are verified server-side; no default, hard-coded, or weak secrets",
	"V3 Session management: session tokens are random, protected, invalidated on logout and expiry",
	"V4 Access control: every endpoint/action checks authorization server-side, deny by default (no IDOR)",
	"V5 Validation, sanitization and encoding: all input validated; output encoded; no injection (SQL, OS, template)",
	"V6 Stored cryptography: sensitive data at rest uses approved algorithms; keys are not in the code",
	"V7 Error handling and logging: errors reveal no internals; security events are logged without secrets",
	"V8 Data protection: personal/sensitive data is minimized, not cached or logged, and removable",
	"V9 Communication: TLS for all external traffic; certificates validated",
	"V10 Malicious code: no backdoors, time bombs, or unexplained network calls; dependencies are trusted",
	"V11 Business logic: flows cannot be abused (limits, ordering, replay, race conditions)",
	"V12 Files and resources: uploads are type/size limited and stored safely; no path traversal or SSRF",
	"V13 API and web service: API inputs/outputs are schema-validated; CORS and rate limits are deliberate",
	"V14 Configuration: secure defaults, security headers, no debug in production, dependencies up to date",
}

// AddSecurityChecklist appends the ASVS checklist to a task (idempotent).
func AddSecurityChecklist(p *project.Project, id string) (*task.Task, bool, error) {
	var t *task.Task
	added := false
	err := WithLock(p, func() error {
		var err error
		if t, err = findTask(p, id); err != nil {
			return err
		}
		if _, ok := doc.Section(t.Body, ASVSSection); ok {
			return nil
		}
		var b strings.Builder
		b.WriteString("<!-- Check each item when verified, or write \"N/A: <reason>\" after the text and check it. -->\n")
		for _, it := range asvsItems {
			b.WriteString("- [ ] " + it + "\n")
		}
		t.Body = doc.SetSection(t.Body, ASVSSection, b.String())
		t.Updated = task.Now()
		added = true
		return task.Save(p, t)
	})
	return t, added, err
}

var checkItem = regexp.MustCompile(`^[-*] \[( |x|X)\] (.+)$`)

// securityOpen returns (section present, unchecked item numbers).
func securityOpen(t *task.Task) (bool, []string) {
	sec, ok := doc.Section(t.Body, ASVSSection)
	if !ok {
		return false, nil
	}
	var open []string
	n := 0
	for _, l := range strings.Split(sec, "\n") {
		if m := checkItem.FindStringSubmatch(l); m != nil {
			n++
			if m[1] == " " {
				open = append(open, fmt.Sprint(n))
			}
		}
	}
	return true, open
}

// ---------- audit ----------

// AuditResult is the outcome of aitk audit.
type AuditResult struct {
	task.Audit
	Task     string            `json:"task,omitempty"`
	Tasks    *AuditTasksResult `json:"fix_tasks,omitempty"`
	Warnings []string          `json:"-"`
}

// Audit runs the security scanners and records evidence on the active task.
func Audit(p *project.Project, stream bool) (*AuditResult, error) {
	tree := profile.Fingerprint(p)
	a, warns := audit.Run(p, stream)
	a.Tree = tree
	res := &AuditResult{Audit: *a, Warnings: warns}
	if id := session.Load(p).Active(); id != "" {
		if err := WithLock(p, func() error {
			t, err := task.Find(p, id)
			if err != nil {
				return nil
			}
			if t.Evidence == nil {
				t.Evidence = &task.Evidence{}
			}
			c := *a
			t.Evidence.Audit = &c
			t.Updated = task.Now()
			res.Task = t.ID
			return task.Save(p, t)
		}); err != nil {
			return res, err
		}
	}
	if !a.Passed {
		var failed []string
		for _, r := range a.Results {
			if r.ExitCode != 0 {
				failed = append(failed, r.Name)
			}
		}
		return res, apperr.New("E_AUDIT_FAILED", apperr.ExitRuntime, "fix or upgrade what the scanners report, then run: aitk audit",
			"security audit found problems: %s", strings.Join(failed, ", "))
	}
	return res, nil
}

// ---------- UAT ----------

// AgentInEnv names the AI agent running this process, or "" for a person. An explicit
// AITK_TOOL=human cannot override agent environment variables.
func AgentInEnv() string {
	for _, v := range []struct{ env, name string }{
		{"CLAUDECODE", "claude-code"}, {"CODEX_SESSION_ID", "codex"}, {"CODEX_SANDBOX", "codex"}, {"CODEX_VERSION", "codex"},
		{"GEMINI_CLI", "gemini-cli"}, {"OPENCODE", "opencode"}, {"CURSOR_AGENT", "cursor"},
	} {
		if os.Getenv(v.env) != "" {
			return v.name
		}
	}
	if t := os.Getenv("AITK_TOOL"); t != "" && t != "human" {
		return t
	}
	if os.Getenv(RunEnv) != "" { // started by aitk run, whatever the tool
		return "aitk-run-agent"
	}
	return ""
}

func findTask(p *project.Project, id string) (*task.Task, error) {
	if id == "" {
		id = session.Load(p).Active()
		if id == "" {
			return nil, apperr.NoActiveTask()
		}
	}
	t, err := task.Find(p, id)
	if err != nil {
		return nil, apperr.TaskNotFound(id)
	}
	return t, nil
}

// UATScript writes docs/ai/uat/<id>.md: a test script for a person, from the criteria.
func UATScript(p *project.Project, id string, force bool) (string, error) {
	t, err := findTask(p, id)
	if err != nil {
		return "", err
	}
	rel := filepath.ToSlash(filepath.Join(project.DocsAI, "uat", t.ID+".md"))
	if fsx.Exists(p.Path(rel)) && !force {
		return rel, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# User acceptance test: %s — %s\n\n", t.ID, t.Title)
	b.WriteString("Tester: \nDate: \nEnvironment / version: \n\n")
	b.WriteString("For each scenario: perform it as a user would, then mark Pass or Fail and note what you saw.\n\n")
	for i, c := range t.Criteria() {
		fmt.Fprintf(&b, "## Scenario %d\n\n%s\n\n- [ ] Pass\n- [ ] Fail\n\nObserved: \n\n", i+1, c.Text)
	}
	b.WriteString("## Decision\n\nAccept: `aitk uat accept " + t.ID + " --note \"…\"`\nReject: `aitk uat reject " + t.ID + " --reason \"…\"`\n")
	return rel, fsx.WriteFile(p.Path(rel), []byte(b.String()), 0o644)
}

// UATDecision is the outcome of accept or reject.
type UATDecision struct {
	Task    string `json:"task"`
	Status  string `json:"status"`
	UAT     string `json:"uat"`
	Missing string `json:"missing,omitempty"` // what still blocks done after acceptance
}

func personName(p *project.Project, by string) string {
	if by != "" {
		return by
	}
	if n, _ := gitx.Run(p.Root, "config", "user.name"); n != "" {
		return n
	}
	return "user"
}

// UATAccept records a person's acceptance and completes the task when nothing else blocks it.
func UATAccept(p *project.Project, id, by, note string) (*UATDecision, error) {
	if a := AgentInEnv(); a != "" {
		return nil, apperr.New("E_UAT_AGENT", apperr.ExitGate, "ask the user to run: aitk uat accept <id> (in their own terminal)",
			"user acceptance must be given by a person, not by an AI agent (%s)", a)
	}
	if err := GuardText("uat note", note); err != nil {
		return nil, err
	}
	var res *UATDecision
	err := WithLock(p, func() error {
		t, err := findTask(p, id)
		if err != nil {
			return err
		}
		if t.Status == task.Done || t.Status == task.Cancelled {
			return apperr.Usage("task %s is already %s", t.ID, t.Status)
		}
		if t.Evidence == nil {
			t.Evidence = &task.Evidence{}
		}
		now := task.Now()
		who := personName(p, by)
		t.Evidence.UAT = &task.UAT{Status: "accepted", By: who, At: now, Note: note}
		res = &UATDecision{Task: t.ID, Status: t.Status, UAT: "accepted"}
		prof := t.Profile
		if computed := profile.Compute(p, profile.Diff(p)); !t.ProfilePinned || computed == "strict" {
			prof = computed
		}
		target, derr := dod(p, t, prof, CloseInput{})
		if derr != nil {
			res.Missing = derr.Error()
		} else if target == task.Done {
			t.Status, t.Closed = task.Done, now
			res.Status = task.Done
			syncSource(p, t)
			claims.Release(p, t.ID, true)
			s := session.Load(p)
			if s.Active() == t.ID {
				s.LastTask, s.LastTaskAt = t.ID, now
				s.SetActive("", "", "")
				session.Save(p, s)
			}
		}
		t.Updated = now
		if err := task.Save(p, t); err != nil {
			return err
		}
		body := "## " + t.Title + "\n\nAccepted by " + who + "."
		if note != "" {
			body += " " + note
		}
		_, err = handoff.Write(p, handoff.Meta{Task: t.ID, At: now, By: "human", Outcome: map[bool]string{true: "done", false: "progress"}[res.Status == task.Done], TaskStatus: t.Status}, body+"\n")
		return err
	})
	return res, err
}

// UATReject records a rejection and sends the task back to in_progress.
func UATReject(p *project.Project, id, by, reason string) (*UATDecision, error) {
	if a := AgentInEnv(); a != "" {
		return nil, apperr.New("E_UAT_AGENT", apperr.ExitGate, "ask the user to run: aitk uat reject <id> --reason \"…\"",
			"user acceptance decisions are made by a person, not by an AI agent (%s)", a)
	}
	if strings.TrimSpace(reason) == "" {
		return nil, apperr.Usage("--reason is required")
	}
	if err := GuardText("uat reason", reason); err != nil {
		return nil, err
	}
	var res *UATDecision
	err := WithLock(p, func() error {
		t, err := findTask(p, id)
		if err != nil {
			return err
		}
		if t.Evidence == nil {
			t.Evidence = &task.Evidence{}
		}
		now := task.Now()
		who := personName(p, by)
		t.Evidence.UAT = &task.UAT{Status: "rejected", By: who, At: now, Note: reason}
		t.Status, t.Updated = task.InProgress, now
		sec := ""
		if s, ok := doc.Section(t.Body, "Notes"); ok {
			sec = s + "\n"
		}
		t.Body = doc.SetSection(t.Body, "Notes", sec+"- "+now+" UAT rejected by "+who+": "+reason)
		if err := task.Save(p, t); err != nil {
			return err
		}
		res = &UATDecision{Task: t.ID, Status: t.Status, UAT: "rejected"}
		_, err = handoff.Write(p, handoff.Meta{Task: t.ID, At: now, By: "human", Outcome: "progress", TaskStatus: t.Status,
			Next: []string{"Address UAT feedback: " + reason}}, "## "+t.Title+"\n\nUAT rejected by "+who+".\n")
		return err
	})
	return res, err
}
