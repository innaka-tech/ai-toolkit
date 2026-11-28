package brief

import (
	"fmt"
	"path"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/profile"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/slop"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
)

// Persona is a persona the active task needs, picked from its tags, risk profile, and changed files.
type Persona struct {
	Name string `json:"name"`
	Why  string `json:"why"`
	Run  string `json:"run"`
}

// characterRules turn [assistant] character and talk into brief rules, so every agent session
// gets them without a slash command.
func characterRules(c project.Config) []string {
	var r []string
	switch c.CharacterMode() {
	case "lite":
		r = append(r, "Character lite: build what was asked; name a smaller option in one line and let the user choose (aitk standards character).")
	case "full":
		r = append(r, "Smallest change that fully works; never cut validation, error handling, security, or what was asked (aitk standards character).")
	case "ultra":
		r = append(r, "Character ultra: challenge the request first and push back, with reasons, on any part the need does not justify; then the smallest change that fully works, never cutting safety (aitk standards character).")
	}
	if c.CharacterMode() != "off" {
		r = append(r, "Correct defects you find: fix in scope test-first, otherwise file a task. End with Skipped: and Risk: lines.")
	}
	switch c.TalkMode() {
	case "lite":
		r = append(r, "Talk lite: terse full sentences, one line per point, no filler. Never shorten code, commands, paths, errors, warnings, questions for the user, or Skipped/Risk.")
	case "caveman":
		r = append(r, "Talk caveman: short blunt sentences, no filler, fragments fine. Never shorten code, commands, paths, errors, warnings, questions for the user, or Skipped/Risk.")
	}
	return r
}

// pickPersonas chooses the personas the active task needs. Signals are deliberately simple and
// visible (tags, profile, file patterns) so a person can predict and override them.
func pickPersonas(p *project.Project, t *task.Task, prof string, changed []profile.Change) ([]Persona, []string) {
	if t == nil {
		return []Persona{{Name: "aitk-team", Why: "no active task: find the SDLC phase of the request", Run: "aitk prompt aitk-team <request>"}}, []string{"sdlc"}
	}
	var out []Persona
	topics := []string{"character"}
	seen := map[string]bool{}
	add := func(name, why, scope string, std ...string) {
		if seen[name] {
			return
		}
		seen[name] = true
		out = append(out, Persona{Name: name, Why: why, Run: strings.TrimSpace("aitk prompt " + name + " " + scope)})
		for _, s := range std {
			if !contains(topics, s) {
				topics = append(topics, s)
			}
		}
	}
	tags := map[string]bool{}
	for _, tg := range t.Tags {
		tags[strings.ToLower(tg)] = true
	}
	var mig, ui, ops, api, code, tests bool
	for _, c := range changed {
		p := strings.ToLower(c.Path)
		base, ext := path.Base(p), path.Ext(p)
		switch {
		case isTest(p, base):
			tests = true
		case strings.Contains(p, "migration") || ext == ".sql" || base == "schema.prisma":
			mig = true
		case ext == ".tsx" || ext == ".jsx" || ext == ".vue" || ext == ".svelte" || ext == ".css" || ext == ".scss" || ext == ".html" || strings.HasSuffix(p, ".blade.php"):
			ui = true
		case strings.HasPrefix(base, "dockerfile") || strings.Contains(base, "compose") && (ext == ".yml" || ext == ".yaml") ||
			strings.HasPrefix(p, ".github/workflows/") || base == ".gitlab-ci.yml" || base == "jenkinsfile" || ext == ".tf" || strings.Contains(p, "helm/") || strings.Contains(p, "k8s/"):
			ops = true
		case strings.Contains(base, "openapi") || strings.Contains(base, "swagger") || strings.HasPrefix(p, "routes/"):
			api = true
		}
		if ext != ".md" && !isTest(p, base) {
			code = true
		}
	}
	if tags["bug"] {
		add("aitk-fix", "bug task: failing test first, root cause, regression test", t.ID, "testing")
	}
	if tags["security"] || prof == "strict" {
		add("aitk-security", "security-tagged or strict task: threat model and OWASP review before close", t.ID+" fix", "security")
	}
	if mig {
		add("aitk-dba", "migrations or SQL changed: integrity, rollback path, locking, indexes", "fix", "database")
	}
	if ui {
		add("aitk-ux", "UI files changed: WCAG 2.1 AA and usability", "fix", "ux")
	}
	if ops {
		add("aitk-devops", "deploy/CI/infrastructure files changed: rollback, health checks, secrets", "fix", "operations")
	}
	if api {
		add("aitk-architect", "API contract or routes changed: compatibility and versioning", "API change in "+t.ID, "api")
	}
	if code && !tests && prof != "lite" {
		add("aitk-qa", "code changed without a test change: add the missing tests", "fix", "testing")
	}
	if tags["spike"] || tags["idea"] || tags["creative"] || tags["design"] {
		add("aitk-creative", "spike/idea/design task: several different options, explicit criteria, a small proof", t.Title)
	}
	paths := make([]string, 0, len(changed))
	for _, c := range changed {
		paths = append(paths, c.Path)
	}
	if hits := slop.Scan(p, paths); len(hits) > 0 {
		add("aitk-deslop", fmt.Sprintf("%d AI-slop hit(s), first %s:%d [%s]", len(hits), hits[0].File, hits[0].Line, hits[0].Rule), "fix", "anti-slop")
	}
	if prof == "strict" {
		topics = append(topics, "review")
	}
	return out, topics
}

func isTest(p, base string) bool {
	return strings.Contains(base, "_test.") || strings.HasPrefix(base, "test_") || strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.") || strings.Contains(p, "/tests/") || strings.HasPrefix(p, "tests/") || strings.Contains(p, "/test/")
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// reviewRules tell the agent to review until a pass finds 0 before moving on, and to stop and
// ask when the passes are not converging.
func reviewRules(c project.Config, t *task.Task, prof string) []string {
	var r []string
	if t == nil {
		return nil // no task: nothing to review or finish yet (a quick lite close is exempt)
	}
	started := t.Review != nil && len(t.Review.Passes) > 0 && t.Review.Passes[len(t.Review.Passes)-1].Findings > 0
	if prof != "strict" && (c.ReviewRequired(prof) || (c.ReviewLoop() && started)) { // lite too once a pass found something
		r = append(r, "Review until clean: review your change, record aitk review pass --findings N, fix what it found, and repeat until a pass finds 0. Only then close.")
	}
	if c.ReviewLoop() && prof == "strict" {
		r = append(r, "Strict work: review your own change until a pass finds 0 and close again; then another tool records the independent pass.")
	}
	if c.OneTaskAtATime() {
		r = append(r, "One task at a time: finish this task (or aitk task block <id> --reason \"…\") before starting another.")
	}
	if t != nil && t.Review != nil {
		ps := t.Review.Passes
		if n := len(ps); n >= 3 && ps[n-1].Findings > 0 && ps[n-1].Findings >= ps[n-2].Findings {
			r = append(r, fmt.Sprintf("Review is not converging (%d, %d, %d findings in the last passes): stop and ask the user whether to split the task or change the approach.", ps[n-3].Findings, ps[n-2].Findings, ps[n-1].Findings))
		}
	}
	return r
}
