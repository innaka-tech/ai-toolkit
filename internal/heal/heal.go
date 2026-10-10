// Package heal repairs aitk's own files so a project keeps working after agents or merges
// damage them. Repairs never discard content: anything that cannot be kept in place is
// moved to docs/ai/_legacy/quarantine/ and reported.
package heal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/innaka-tech/ai-toolkit/v2/internal/doc"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/migrate"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/schema"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
	"github.com/innaka-tech/ai-toolkit/v2/internal/textx"
)

// Action is one repair.
type Action struct {
	File   string `json:"file"`
	Action string `json:"action"` // repaired | restored | resolved | quarantined
	Detail string `json:"detail"`
}

func (a Action) String() string { return fmt.Sprintf("%s %s: %s", a.Action, a.File, a.Detail) }

// Problems reports what Run would repair, without writing.
func Problems(p *project.Project) []Action { return run(p, true) }

// Run repairs conflict markers and invalid task files.
func Run(p *project.Project) []Action { return run(p, false) }

func run(p *project.Project, dry bool) []Action {
	var out []Action
	out = append(out, resolveConflicts(p, dry)...)
	out = append(out, repairTasks(p, dry)...)
	return out
}

var openRe = regexp.MustCompile(`^<{7}( |$)`)

// hasConflict reports whether s contains a git conflict hunk: a "<<<<<<<" line, then "=======",
// then ">>>>>>>", outside fenced code. A Markdown setext underline ("=======") alone is not one.
func hasConflict(s string) bool {
	_, _, ok := conflictSides(s)
	return ok
}

func hasConflictBytes(b []byte) bool { return hasConflict(string(b)) }

// conflictSides returns the file with every conflict hunk replaced by "ours" and by "theirs";
// ok is false when there is no complete hunk.
func conflictSides(s string) (ours, theirs string, ok bool) {
	var o, t strings.Builder
	state, hunks, fence := 0, 0, false // state: 0 common, 1 ours, 2 theirs, 3 diff3 base
	for _, line := range strings.SplitAfter(s, "\n") {
		trim := strings.TrimRight(line, "\r\n")
		if state == 0 && strings.HasPrefix(strings.TrimSpace(trim), "```") {
			fence = !fence
		}
		switch {
		case !fence && openRe.MatchString(trim) && state == 0:
			state = 1
		case strings.HasPrefix(trim, "|||||||") && state == 1:
			state = 3 // diff3 base: skip
		case trim == "=======" && (state == 1 || state == 3):
			state = 2
		case strings.HasPrefix(trim, ">>>>>>>") && state == 2:
			state = 0
			hunks++
		default:
			switch state {
			case 0:
				o.WriteString(line)
				t.WriteString(line)
			case 1:
				o.WriteString(line)
			case 2:
				t.WriteString(line)
			}
		}
	}
	return o.String(), t.String(), state == 0 && hunks > 0
}

// union keeps every line from both sides (for append-only lists such as knowledge).
func union(s string) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(s, "\n") {
		trim := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(trim, "<<<<<<<") || strings.HasPrefix(trim, ">>>>>>>") || strings.HasPrefix(trim, "|||||||") || trim == "=======" {
			continue
		}
		b.WriteString(line)
	}
	return b.String()
}

func resolveConflicts(p *project.Project, dry bool) []Action {
	var out []Action
	root := p.Path(project.DocsAI)
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == "_legacy" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil || !hasConflictBytes(b) {
			return nil
		}
		rel, _ := filepath.Rel(p.Root, path)
		rel = filepath.ToSlash(rel)
		ours, theirs, ok := conflictSides(string(b))
		if !ok {
			return nil // not a well-formed conflict; leave for a human (doctor reports schema errors)
		}
		if strings.HasPrefix(rel, project.TasksDir+"/") {
			keep, other, why := pickTask(ours, theirs)
			out = append(out, Action{File: rel, Action: "resolved", Detail: "merge conflict: kept the " + why + " version; the other is in quarantine"})
			if !dry {
				quarantine(p, rel, []byte(other), "conflict-other")
				fsx.WriteFile(path, []byte(keep), 0o644)
			}
			return nil
		}
		out = append(out, Action{File: rel, Action: "resolved", Detail: "merge conflict: kept lines from both sides"})
		if !dry {
			fsx.WriteFile(path, []byte(union(string(b))), 0o644)
		}
		return nil
	})
	return out
}

// pickTask chooses between two versions of a task: the more recently updated one,
// then the one further along.
func pickTask(a, b string) (keep, other, why string) {
	ma, mb := metaOf(a), metaOf(b)
	switch {
	case ma.Updated > mb.Updated:
		return a, b, "more recently updated (ours)"
	case mb.Updated > ma.Updated:
		return b, a, "more recently updated (theirs)"
	case rank(mb.Status) > rank(ma.Status):
		return b, a, "further along (theirs)"
	}
	return a, b, "current branch's (ours)"
}

func metaOf(s string) task.Meta {
	var m task.Meta
	if front, _, ok := doc.Split(s); ok {
		yaml.Unmarshal([]byte(front), &m)
	}
	return m
}

func rank(s string) int {
	return map[string]int{task.Todo: 1, task.Blocked: 2, task.InProgress: 3, task.Implemented: 4, task.InReview: 5, task.Done: 6, task.Cancelled: 0}[s]
}

func quarantine(p *project.Project, rel string, content []byte, tag string) string {
	name := time.Now().UTC().Format("20060102T150405Z") + "-" + tag + "-" + filepath.Base(rel)
	dst := filepath.ToSlash(filepath.Join(project.LegacyDir, "quarantine", name))
	fsx.WriteFile(p.Path(dst), content, 0o644)
	return dst
}

var (
	h1Re     = regexp.MustCompile(`(?m)^# +(.+)$`)
	statusRe = regexp.MustCompile(`(?mi)^\**status:?\**:? *(.+)$`)
)

// repairTasks normalizes task files that fail their schema.
func repairTasks(p *project.Project, dry bool) []Action {
	var out []Action
	entries, _ := os.ReadDir(p.Path(project.TasksDir))
	used := map[string]bool{}
	for _, e := range entries {
		if t, err := task.Load(p, filepath.ToSlash(filepath.Join(project.TasksDir, e.Name()))); err == nil {
			used[strings.ToLower(t.ID)] = true
		}
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(project.TasksDir, e.Name()))
		raw, err := os.ReadFile(p.Path(rel))
		if err != nil || hasConflictBytes(raw) {
			continue // conflicts are handled first; unreadable files are reported by doctor
		}
		if t, err := task.Load(p, rel); err == nil && schema.Validate("task", t.Meta) == nil {
			continue
		}
		a := repairOne(p, rel, string(raw), used, dry)
		out = append(out, a)
	}
	return out
}

func repairOne(p *project.Project, rel, content string, used map[string]bool, dry bool) Action {
	front, body, hasFront := doc.Split(content)
	if !hasFront {
		body = content
	}
	fm := map[string]any{}
	if hasFront && yaml.Unmarshal([]byte(front), &fm) != nil {
		// Unparseable YAML: prefer the last committed version when it is valid.
		if head, err := gitx.RunRaw(p.Root, "show", "HEAD:"+rel); err == nil {
			if f2, b2, ok := doc.Split(string(head)); ok {
				var m task.Meta
				if yaml.Unmarshal([]byte(f2), &m) == nil && schema.Validate("task", m) == nil {
					if !dry {
						q := quarantine(p, rel, []byte(content), "broken")
						_ = b2
						fsx.WriteFile(p.Path(rel), head, 0o644)
						return Action{File: rel, Action: "restored", Detail: "frontmatter was not valid YAML; restored the committed version (your edit is in " + q + ")"}
					}
					return Action{File: rel, Action: "restored", Detail: "frontmatter is not valid YAML; would restore the committed version"}
				}
			}
		}
		fm = map[string]any{}
	}
	var changes []string
	stem := strings.TrimSuffix(filepath.Base(rel), ".md")
	m := task.Meta{}
	str := func(k string) string {
		if v, ok := fm[k]; ok {
			delete(fm, k)
			switch x := v.(type) {
			case string:
				return strings.TrimSpace(x)
			case time.Time:
				return x.UTC().Format("2006-01-02T15:04:05Z")
			case nil:
				return ""
			default:
				return strings.TrimSpace(fmt.Sprint(x))
			}
		}
		return ""
	}
	m.ID = str("id")
	if !task.IDPattern.MatchString(m.ID) {
		cand := strings.SplitN(stem, "-", 3)
		guess := cand[0]
		if len(cand) > 1 && strings.EqualFold(cand[0], "T") {
			guess = cand[0] + "-" + cand[1]
		}
		if task.IDPattern.MatchString(guess) && !used[strings.ToLower(guess)] {
			changes = append(changes, fmt.Sprintf("id %q → %q", m.ID, guess))
			m.ID = guess
		} else {
			newID := task.NewID(p)
			changes = append(changes, fmt.Sprintf("id %q → %q", m.ID, newID))
			m.ID = newID
		}
		used[strings.ToLower(m.ID)] = true
	}
	m.Title = str("title")
	if len([]rune(m.Title)) < 3 {
		if h := h1Re.FindStringSubmatch(body); h != nil {
			m.Title = textx.Truncate(strings.TrimSpace(h[1]), 120)
		} else {
			m.Title = "Task " + m.ID
		}
		changes = append(changes, "title restored")
	}
	m.Title = textx.Truncate(m.Title, 120)
	rawStatus := str("status")
	if rawStatus == "" {
		if s := statusRe.FindStringSubmatch(body); s != nil {
			rawStatus = s[1]
		}
	}
	m.Status = rawStatus
	if !contains(task.Statuses, m.Status) {
		m.Status = migrate.StatusMap(rawStatus)
		changes = append(changes, fmt.Sprintf("status %q → %q", rawStatus, m.Status))
	}
	m.Profile = str("profile")
	if m.Profile != "lite" && m.Profile != "standard" && m.Profile != "strict" {
		if m.Profile != "" {
			changes = append(changes, fmt.Sprintf("profile %q → standard", m.Profile))
		}
		m.Profile = "standard"
	}
	if v, ok := fm["profile_pinned"].(bool); ok {
		m.ProfilePinned = v
		delete(fm, "profile_pinned")
	}
	fallback := gitTime(p, rel)
	for _, f := range []struct {
		key string
		dst *string
	}{{"created", &m.Created}, {"updated", &m.Updated}, {"closed", &m.Closed}} {
		v := str(f.key)
		norm := normTime(v)
		if norm == "" && f.key != "closed" {
			norm = fallback
		}
		if norm != v {
			changes = append(changes, fmt.Sprintf("%s %q → %q", f.key, v, norm))
		}
		*f.dst = norm
	}
	m.Goal = str("goal")
	if m.Goal != "" && !regexp.MustCompile(`^G-[A-Za-z0-9.-]{1,16}$`).MatchString(m.Goal) {
		changes = append(changes, "goal moved to legacy")
		fm["goal"], m.Goal = m.Goal, ""
	}
	m.CreatedBy, m.Branch = str("created_by"), str("branch")
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`).MatchString(m.CreatedBy) {
		if m.CreatedBy != "" {
			fm["created_by"] = m.CreatedBy
		}
		m.CreatedBy = ""
	}
	if v, ok := fm["tags"].([]any); ok {
		for _, x := range v {
			m.Tags = append(m.Tags, project.Slug(fmt.Sprint(x), 62))
		}
		delete(fm, "tags")
		m.Tags = dedupe(m.Tags)
	}
	// Structured fields aitk writes itself: keep them if they still validate, else move to legacy.
	for _, k := range []string{"evidence", "review", "paths", "depends_on", "legacy"} {
		v, ok := fm[k]
		if !ok {
			continue
		}
		delete(fm, k)
		trial := m
		b, _ := yaml.Marshal(map[string]any{k: v})
		if yaml.Unmarshal(b, &trial) == nil && schema.Validate("task", trial) == nil {
			m = trial
		} else {
			fm[k] = v
			changes = append(changes, k+" moved to legacy (invalid)")
		}
	}
	if len(fm) > 0 {
		if m.Legacy == nil {
			m.Legacy = map[string]any{}
		}
		keys := make([]string, 0, len(fm))
		for k := range fm {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		unknown := map[string]any{}
		for _, k := range keys {
			unknown[k] = fm[k]
		}
		m.Legacy["unknown_fields"] = unknown
		changes = append(changes, "unknown fields kept under legacy.unknown_fields: "+strings.Join(keys, ", "))
	}
	if err := schema.Validate("task", m); err != nil {
		if !dry {
			q := quarantine(p, rel, []byte(content), "unrepairable")
			os.Remove(p.Path(rel))
			return Action{File: rel, Action: "quarantined", Detail: "could not be repaired (" + err.Error() + "); moved to " + q}
		}
		return Action{File: rel, Action: "quarantined", Detail: "cannot be repaired: " + err.Error()}
	}
	if !dry {
		out, _ := doc.Join(m, body)
		fsx.WriteFile(p.Path(rel), out, 0o644)
	}
	if len(changes) == 0 {
		changes = []string{"normalized frontmatter"}
	}
	return Action{File: rel, Action: "repaired", Detail: strings.Join(changes, "; ")}
}

func gitTime(p *project.Project, rel string) string {
	if d := gitx.LastCommitDate(p.Root, rel); d != "" {
		if t, err := time.Parse(time.RFC3339, d); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05Z")
		}
	}
	return task.Now()
}

// normTime converts common timestamp spellings to canonical UTC RFC 3339 ("" when unparseable).
func normTime(v string) string {
	if v == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05Z")
		}
	}
	return ""
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
