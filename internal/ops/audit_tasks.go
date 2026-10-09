package ops

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/audit"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
	"github.com/innaka-tech/ai-toolkit/v2/internal/textx"
)

// AuditTasksResult lists fix tasks created from audit findings.
type AuditTasksResult struct {
	Created []string `json:"created"`
	Open    []string `json:"already_open"` // findings that already have an open task
}

var nonID = regexp.MustCompile(`[^A-Za-z0-9]+`)

// AuditTasks turns the findings of a failed audit into fix tasks: one per vulnerable dependency
// (from osv-scanner, with the version that fixes every advisory) and one per other failing
// scanner. Findings that already have an open task are not duplicated.
func AuditTasks(p *project.Project, r *AuditResult) (*AuditTasksResult, error) {
	out := &AuditTasksResult{Created: []string{}, Open: []string{}}
	var items []NewTaskInput
	var ids []string
	for _, res := range r.Results {
		if res.ExitCode == 0 {
			continue
		}
		if strings.HasPrefix(res.Name, "osv-scanner") {
			if _, err := exec.LookPath("osv-scanner"); err == nil {
				pkgs, err := audit.OSVPackages(p.Root)
				if err != nil {
					return nil, err
				}
				for _, v := range pkgs {
					ids = append(ids, auditID("VULN", v.Ecosystem+"-"+v.Name))
					items = append(items, vulnTask(v))
				}
				continue
			}
		}
		ids = append(ids, auditID("AUDIT", res.Name))
		items = append(items, NewTaskInput{
			Title:     textx.Truncate("Fix the findings of "+res.Name, 120),
			Objective: "The security audit failed in " + res.Name + ". Fix every finding (or record why one is a false positive), then run aitk audit.\n\n```\n" + textx.Truncate(res.Summary, 1500) + "\n```",
			Criteria:  []string{"Given the fix, when aitk audit runs, then " + res.Name + " passes"},
			Tags:      []string{"security", "bug"},
		})
	}
	tasks, _ := task.List(p)
	seen := map[string]bool{}
	for i, in := range items {
		id := ids[i]
		if seen[strings.ToLower(id)] {
			continue
		}
		seen[strings.ToLower(id)] = true
		open, n := false, 0
		for _, t := range tasks {
			if strings.EqualFold(t.ID, id) || strings.HasPrefix(strings.ToLower(t.ID), strings.ToLower(id)+"-") {
				n++
				if t.Status != task.Done && t.Status != task.Cancelled {
					open = true
					out.Open = append(out.Open, t.ID)
					break
				}
			}
		}
		if open {
			continue
		}
		if n > 0 { // fixed before, vulnerable again
			id = fmt.Sprintf("%s-%d", id, n+1)
		}
		in.ID = id
		t, err := TaskNew(p, in)
		if err != nil && strings.Contains(in.Objective, "```") {
			// The scanner output may hold text the credential guard refuses; keep the task without it.
			in.Objective = in.Objective[:strings.Index(in.Objective, "```")] + "Run aitk audit to see the findings."
			t, err = TaskNew(p, in)
		}
		if err != nil {
			return out, err
		}
		out.Created = append(out.Created, t.ID)
	}
	return out, nil
}

func auditID(prefix, name string) string {
	s := strings.Trim(nonID.ReplaceAllString(name, "-"), "-")
	if max := 32 - len(prefix) - 4; len(s) > max { // room for "-" and a "-NN" suffix
		s = strings.TrimRight(s[:max], "-")
	}
	return prefix + "-" + s
}

func vulnTask(v audit.VulnPackage) NewTaskInput {
	var upgrades, found []string
	for _, vv := range v.Versions {
		to := "no fixed version published yet"
		if vv.FixedIn != "" {
			to = vv.FixedIn + " or later"
		}
		upgrades = append(upgrades, vv.Version+" → "+to)
		found = append(found, vv.Version+" in "+strings.Join(vv.Sources, ", "))
	}
	n := len(v.Advisories)
	adv := plural(n, "advisory", "advisories")
	title := fmt.Sprintf("Upgrade %s %s (%s)", v.Name, strings.Join(upgrades, "; "), adv)
	if len([]rune(title)) > 120 {
		title = fmt.Sprintf("Upgrade %s past %s (%d versions)", v.Name, adv, len(v.Versions))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s package %s has %s", v.Ecosystem, v.Name, adv)
	if v.MaxSeverity > 0 {
		fmt.Fprintf(&b, " (highest CVSS %.1f)", v.MaxSeverity)
	}
	fmt.Fprintf(&b, ".\n\nFound: %s.\nUpgrade: %s.\nAdvisories: %s.\n\nUpgrade it (directly, or the dependency that pulls it in), run the check to catch breaking changes, then run aitk audit.",
		strings.Join(found, "; "), strings.Join(upgrades, "; "), strings.Join(v.Advisories, ", "))
	return NewTaskInput{
		Title:     title,
		Objective: b.String(),
		Criteria: []string{
			fmt.Sprintf("Given the lockfiles, when aitk audit runs, then osv-scanner reports no advisory for %s", v.Name),
			"Given the upgrade, when aitk check runs, then it passes",
		},
		Tags: []string{"security", "dependencies"},
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
