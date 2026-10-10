package ops

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/innaka-tech/ai-toolkit/v2/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/v2/internal/doc"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
	"github.com/innaka-tech/ai-toolkit/v2/internal/textx"
)

// shortID builds an ID from a prefix and name parts. The last part (the task or story number)
// is always kept whole; earlier parts are shortened to fit 32 characters, so tasks of one plan
// never collapse into the same ID.
func shortID(prefix string, parts ...string) string {
	var segs []string
	for _, p := range parts {
		s := strings.ToUpper(project.Slug(p, 40))
		if s == "UNTITLED" || s == "" {
			continue
		}
		segs = append(segs, s)
	}
	if len(segs) == 0 {
		return prefix
	}
	last := segs[len(segs)-1]
	head := prefix
	if len(segs) > 1 {
		head += "-" + strings.Join(segs[:len(segs)-1], "-")
	}
	if len(last) > 20 {
		last = strings.TrimRight(last[:20], "-")
	}
	if room := 32 - len(last) - 1; len(head) > room {
		head = strings.TrimRight(head[:room], "-")
	}
	return head + "-" + last
}

// ---------- Superpowers (obra/superpowers writing-plans) ----------

var (
	spTask = regexp.MustCompile(`(?m)^### Task (\d+): *(.+)$`)
	spStep = regexp.MustCompile(`^[-*] \[( |x|X)\] (.+)$`)
	h1     = regexp.MustCompile(`(?m)^# +(.+)$`)
	dated  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-`)
)

// ImportSuperpowers imports Superpowers implementation plans (docs/superpowers/plans/*.md):
// each "### Task N: Name" becomes a task; its "- [ ] **Step k: …**" lines become criteria.
func ImportSuperpowers(p *project.Project, paths []string, dry bool) (*ImportResult, error) {
	if len(paths) == 0 {
		paths, _ = filepath.Glob(p.Path("docs", "superpowers", "plans", "*.md"))
		if len(paths) == 0 {
			return nil, apperr.New("E_IMPORT", apperr.ExitUsage, "aitk import superpowers <plan.md>", "no plans found in docs/superpowers/plans/")
		}
	}
	var items []importItem
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, apperr.New("E_IMPORT", apperr.ExitUsage, "check the path", "%v", err)
		}
		text := string(b)
		plan := strings.TrimSuffix(filepath.Base(path), ".md")
		planTitle := plan
		if m := h1.FindStringSubmatch(text); m != nil {
			planTitle = strings.TrimSpace(strings.TrimSuffix(m[1], " Implementation Plan"))
		}
		rel, _ := filepath.Rel(p.Root, path)
		locs := spTask.FindAllStringSubmatchIndex(text, -1)
		for i, loc := range locs {
			end := len(text)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			num, name := text[loc[2]:loc[3]], strings.TrimSpace(text[loc[4]:loc[5]])
			section := text[loc[1]:end]
			if j := strings.Index(section, "\n## "); j >= 0 {
				section = section[:j]
			}
			it := importItem{id: shortID("SP", dated.ReplaceAllString(plan, ""), num), title: name, tags: []string{"superpowers"},
				legacy: map[string]any{"source": "superpowers", "file": filepath.ToSlash(rel), "task": num}}
			done := 0
			var files []string
			for _, l := range strings.Split(section, "\n") {
				if m := spStep.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
					step := strings.ReplaceAll(m[2], "**", "")
					it.criteria = append(it.criteria, textx.Truncate(step, 300))
					if m[1] != " " {
						done++
					}
				} else if strings.HasPrefix(strings.TrimSpace(l), "- Create:") || strings.HasPrefix(strings.TrimSpace(l), "- Modify:") || strings.HasPrefix(strings.TrimSpace(l), "- Test:") {
					files = append(files, strings.TrimSpace(l))
				}
			}
			it.done = len(it.criteria) > 0 && done == len(it.criteria)
			it.objective = "Part of the plan \"" + planTitle + "\" (" + filepath.ToSlash(rel) + ")."
			if len(files) > 0 {
				it.objective += "\n\nFiles:\n" + strings.Join(files, "\n")
			}
			items = append(items, it)
		}
	}
	return importItems(p, "superpowers", items, dry)
}

// ---------- BMAD Method (bmad-code-org/BMAD-METHOD tickets) ----------

var (
	bmCriterion = regexp.MustCompile(`^(\d+)\. +(.+)$`)
	bmVerify    = regexp.MustCompile(`(?m)^Verify: *(.+)$`)
)

// bmadStatus maps a BMAD plan status to an aitk task status.
func bmadStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "in-progress":
		return task.InProgress
	case "in-review", "built":
		return task.Implemented
	case "done":
		return task.Done
	case "blocked":
		return task.Blocked
	case "dropped":
		return task.Cancelled
	}
	return task.Todo // draft, ready-for-dev, none
}

// ImportBMAD imports BMAD tickets (story, bug, spike files with YAML frontmatter). Criteria come
// from "## Acceptance Criteria" (numbered Given/When/Then items, or a Verify: line); status from
// the sibling <type>-<slug>-plan.md; risk: high pins the strict profile.
func ImportBMAD(p *project.Project, paths []string, dry bool) (*ImportResult, error) {
	var files []string
	if len(paths) == 0 {
		paths = []string{p.Root}
	}
	for _, root := range paths {
		info, err := os.Stat(root)
		if err != nil {
			return nil, apperr.New("E_IMPORT", apperr.ExitUsage, "check the path", "%v", err)
		}
		if !info.IsDir() {
			files = append(files, root)
			continue
		}
		filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.IsDir() {
				switch fi.Name() {
				case ".git", "node_modules", "vendor", ".aitk":
					return filepath.SkipDir
				}
				if rel, _ := filepath.Rel(p.Root, path); filepath.ToSlash(rel) == project.DocsAI {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, "-plan.md") {
				files = append(files, path)
			}
			return nil
		})
	}
	var items []importItem
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		front, body, ok := doc.Split(string(b))
		if !ok {
			continue
		}
		var fm map[string]any
		if yaml.Unmarshal([]byte(front), &fm) != nil {
			continue
		}
		typ := str(fm["type"])
		if typ != "story" && typ != "bug" && typ != "spike" && typ != "task" {
			continue
		}
		title := strings.Trim(str(fm["title"]), `"' `)
		if title == "" || strings.HasPrefix(title, "[") {
			if m := h1.FindStringSubmatch(body); m != nil {
				title = strings.TrimSpace(m[1])
			}
		}
		stem := strings.TrimSuffix(filepath.Base(path), ".md")
		parent := strings.TrimPrefix(strings.TrimPrefix(str(fm["parent"]), "epic-"), "initiative-")
		bid := ""
		switch v := fm["id"].(type) {
		case int:
			bid = itoa(v)
		case string:
			bid = v
		}
		id := shortID("BM", parent, bid)
		if bid == "" {
			id = shortID("BM", strings.TrimPrefix(stem, typ+"-"))
		}
		status := str(fm["status"])
		if pb, err := os.ReadFile(strings.TrimSuffix(path, ".md") + "-plan.md"); err == nil {
			if pf, _, ok := doc.Split(string(pb)); ok {
				var pm map[string]any
				if yaml.Unmarshal([]byte(pf), &pm) == nil && str(pm["status"]) != "" {
					status = str(pm["status"])
				}
			}
		}
		rel, _ := filepath.Rel(p.Root, path)
		it := importItem{id: id, title: title, tags: []string{"bmad", typ},
			legacy: map[string]any{"source": "bmad", "file": filepath.ToSlash(rel), "type": typ, "parent": str(fm["parent"]), "bmad_id": bid, "bmad_status": status}}
		it.status = bmadStatus(status)
		if strings.EqualFold(str(fm["risk"]), "high") {
			it.profile = "strict"
		}
		if d, ok := doc.Section(body, "Description"); ok {
			it.objective = strings.TrimSpace(d)
		}
		if ac, ok := doc.Section(body, "Acceptance Criteria"); ok {
			it.criteria = bmadCriteria(ac)
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		return nil, apperr.New("E_IMPORT", apperr.ExitUsage, "aitk import bmad <tickets folder or file>", "no BMAD tickets (type: story, bug, spike) found")
	}
	return importItems(p, "bmad", items, dry)
}

func bmadCriteria(ac string) []string {
	var out []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, textx.Truncate(strings.Join(cur, " "), 500))
			cur = nil
		}
	}
	for _, l := range strings.Split(ac, "\n") {
		t := strings.TrimSpace(strings.ReplaceAll(l, "**", ""))
		switch {
		case t == "" || strings.HasPrefix(t, "<!--") || strings.HasPrefix(t, "["):
		case bmCriterion.MatchString(t):
			flush()
			m := bmCriterion.FindStringSubmatch(t)
			cur = []string{m[2] + ":"}
		case len(cur) > 0:
			cur = append(cur, t)
		}
	}
	flush()
	if len(out) == 0 {
		if m := bmVerify.FindStringSubmatch(ac); m != nil {
			out = append(out, "Verify: "+strings.TrimSpace(m[1]))
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	if neg {
		d = append([]byte{'-'}, d...)
	}
	return string(d)
}
