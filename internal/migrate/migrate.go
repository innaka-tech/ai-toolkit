// Package migrate converts v1 ai-toolkit projects to v2 (docs/spec/migration-v1.md).
// It is lossless (every rewritten file is copied to docs/ai/_legacy/ first) and idempotent.
package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v3"

	"github.com/innaka-tech/ai-toolkit/v2/internal/agentsmd"
	"github.com/innaka-tech/ai-toolkit/v2/internal/compat"
	"github.com/innaka-tech/ai-toolkit/v2/internal/doc"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/handoff"
	"github.com/innaka-tech/ai-toolkit/v2/internal/knowledge"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/schema"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
	"github.com/innaka-tech/ai-toolkit/v2/internal/textx"
)

// Report describes a migration.
type Report struct {
	Project     string         `json:"project"`
	AlreadyV2   bool           `json:"already_v2,omitempty"`
	DryRun      bool           `json:"dry_run"`
	Counts      map[string]int `json:"counts"`
	Warnings    []string       `json:"warnings,omitempty"`
	Written     []string       `json:"written,omitempty"`
	Removed     []string       `json:"removed,omitempty"`
	Legacy      []string       `json:"legacy,omitempty"`
	V1Tokens    int            `json:"v1_tokens"`
	planned     map[string][]byte
	removals    []string
	legacyBytes map[string][]byte
}

var (
	idRE     = task.IDPattern
	dateRE   = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})(?:[T ](\d{2}:\d{2}(?::\d{2})?)Z?)?`)
	statusRE = regexp.MustCompile(`(?mi)^\**status:?\**:? *(.+)$`)
	envRE    = regexp.MustCompile(`^([A-Z_][A-Z0-9_]*)="?(.*?)"?$`)
)

// StatusMap implements the status mapping table.
func StatusMap(raw string) string {
	v := strings.ToLower(strings.TrimSpace(strings.NewReplacer("*", "", "`", "").Replace(raw)))
	for _, m := range []struct{ re, out string }{
		{`^(active|in[_ ]progress|eksekusi)`, task.InProgress},
		{`^implemented`, task.Implemented},
		{`^(review|reviewed|bug[_ ]hunt)`, task.InReview},
		{`^(completed|done|✅)`, task.Done},
		{`^blocked`, task.Blocked},
		{`^cancell?ed`, task.Cancelled},
	} {
		if regexp.MustCompile(m.re).MatchString(v) {
			return m.out
		}
	}
	return task.Todo
}

// Run migrates the project containing dir.
func Run(dir string, dryRun bool) (*Report, error) {
	p, err := project.Repo(dir)
	if err != nil {
		return nil, err
	}
	r := &Report{Project: filepath.Base(p.Root), DryRun: dryRun, Counts: map[string]int{}, planned: map[string][]byte{}, legacyBytes: map[string][]byte{}}
	if !project.IsV1(p.Root) {
		r.AlreadyV2 = fsx.Exists(p.Path(project.ConfigFile))
		if !r.AlreadyV2 {
			r.Warnings = append(r.Warnings, "no v1 project found; use `aitk init`")
		}
		return r, nil
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	used := map[string]bool{}

	// config
	env := map[string]string{}
	if b, err := os.ReadFile(p.Path(".ai-toolkit", "project.env")); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if m := envRE.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
				env[m[1]] = m[2]
			}
		}
	}
	name := project.Slug(firstNonEmpty(env["PROJECT_NAME"], filepath.Base(p.Root)), 62)
	cfg := project.Config{Project: project.ProjectCfg{Name: name, DefaultBranch: env["DEFAULT_BRANCH"]}}
	targets := map[string]string{}
	for _, k := range []string{"default", "staging", "production"} {
		if v := env["DEPLOY_"+strings.ToUpper(k)]; v != "" {
			targets[k] = v
		}
	}
	if len(targets) > 0 {
		cfg.Deploy.Targets = targets
	}
	cfg.Deploy.PostCheck = env["POST_DEPLOY_CHECK"]
	settings := map[string]map[string]any{}
	if v := env["UTEKE_NAMESPACE"]; v != "" {
		settings["uteke"] = map[string]any{"namespace": v}
	}
	if v := env["CODEBASE_PROJECT"]; v != "" {
		settings["codebase-memory"] = map[string]any{"project": v}
	}
	if len(settings) > 0 {
		cfg.Plugins.Settings = settings
	}
	var dropped []string
	for k := range env {
		if k == "PROJECT_ROOT" || strings.HasPrefix(k, "BROWSER_MCP_") {
			dropped = append(dropped, k)
		}
	}
	sort.Strings(dropped)
	if len(dropped) > 0 {
		r.Warnings = append(r.Warnings, "dropped machine-specific config: "+strings.Join(dropped, ", "))
	}
	check := project.DetectCheck(p.Root)
	if check != "" {
		cfg.Check.Cmd = check
	} else {
		r.Warnings = append(r.Warnings, "no check command detected; set [check] cmd in aitk.toml")
	}
	rendered := renderConfig(cfg)
	var raw map[string]any
	if _, err := toml.Decode(string(rendered), &raw); err != nil {
		return nil, err
	}
	if err := schema.Validate("config", raw); err != nil {
		return nil, err
	}
	r.plan(project.ConfigFile, rendered)

	// state
	var st1 map[string]any
	if b, err := os.ReadFile(p.Path(project.StateFile)); err == nil {
		r.keep(p, project.StateFile)
		json.Unmarshal(b, &st1)
	}
	state := map[string]any{"schema_version": 2, "project": map[string]any{"name": name}, "aitk": map[string]any{"migrated_from": 1, "migrated_at": now}}
	if ctx, _ := st1["context"].(string); strings.TrimSpace(ctx) != "" && !strings.Contains(ctx, "update this summary") {
		state["project"].(map[string]any)["summary"] = textx.Truncate(strings.TrimSpace(ctx), 280)
	}
	if err := schema.Validate("state", state); err != nil {
		return nil, err
	}
	sb, _ := json.MarshalIndent(state, "", "  ")
	r.plan(project.StateFile, append(sb, '\n'))

	// tasks
	addTemplate := func(text, src string) bool {
		st := regexp.MustCompile(`(?m)^Status: *(.+)$`).FindStringSubmatch(text)
		tid := regexp.MustCompile(`(?m)^Task ID: *(.+)$`).FindStringSubmatch(text)
		if st == nil || tid == nil {
			return false
		}
		// v1 wrote a placeholder when nothing was active ("Task ID: none", "Status: IDLE").
		switch strings.ToLower(strings.TrimSpace(tid[1])) {
		case "none", "n/a", "-", "":
			return false
		}
		switch strings.ToLower(strings.TrimSpace(st[1])) {
		case "idle", "none":
			return false
		}
		obj, _ := doc.Section(text, "Objective")
		title := textx.Truncate(textx.FirstLine(obj), 120)
		if len([]rune(title)) < 3 {
			title = "Migrated task " + strings.TrimSpace(tid[1])
		}
		created := matchTS(text, "Started At")
		if created == "" {
			created = commitTS(p, src)
		}
		updated := matchTS(text, "Updated At")
		if updated == "" {
			updated = created
		}
		id := newID(used)
		m := task.Meta{ID: id, Title: title, Status: StatusMap(st[1]), Profile: "standard", Created: created, Updated: updated,
			Legacy: map[string]any{"v1_id": strings.TrimSpace(tid[1]), "status": strings.TrimSpace(st[1]), "source": src}}
		if g := regexp.MustCompile(`(?m)^Goal ID: *(G-[A-Za-z0-9.-]{1,16})\s*$`).FindStringSubmatch(text); g != nil {
			m.Goal = g[1]
		}
		if schema.Validate("task", m) != nil {
			return false
		}
		b, _ := doc.Join(m, stripHeader(text))
		r.plan(filepath.ToSlash(filepath.Join(project.TasksDir, id+"-"+project.Slug(title, 48)+".md")), b)
		r.Counts["tasks"]++
		return true
	}
	if b, err := os.ReadFile(p.Path(project.DocsAI, "current-task.md")); err == nil {
		rel := project.DocsAI + "/current-task.md"
		r.keep(p, rel)
		r.removals = append(r.removals, rel) // copied to _legacy; replaced by a generated index
		if !addTemplate(string(b), rel) {
			msg := "current-task.md is free-form: kept in docs/ai/_legacy/, shown by `aitk brief` until tasks exist"
			if regexp.MustCompile(`(?mi)^(Task ID: *(none|n/a|-)\s*$|Status: *idle\s*$)`).MatchString(string(b)) {
				msg = "current-task.md had no active v1 task: kept in docs/ai/_legacy/ (no task created)"
			}
			r.Warnings = append(r.Warnings, msg)
		}
	}
	entries, _ := os.ReadDir(p.Path(project.TasksDir))
	unparsed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(project.TasksDir, e.Name()))
		b, _ := os.ReadFile(p.Path(rel))
		text := string(b)
		front, body, hasFront := doc.Split(text)
		var fm map[string]any
		if hasFront {
			if yaml.Unmarshal([]byte(front), &fm) != nil {
				fm = nil
			}
			var meta task.Meta
			if yaml.Unmarshal([]byte(front), &meta) == nil && schema.Validate("task", meta) == nil {
				used[strings.ToLower(meta.ID)] = true
				continue // already a valid v2 task
			}
		}
		r.keep(p, rel)
		r.removals = append(r.removals, rel)
		if !hasFront && strings.HasSuffix(e.Name(), ".previous.md") {
			if !addTemplate(text, rel) {
				unparsed++
				r.plan(filepath.ToSlash(filepath.Join(project.LegacyDir, "tasks", e.Name())), b)
			}
			continue
		}
		if !hasFront {
			body = text
		}
		stem := strings.TrimSuffix(e.Name(), ".md")
		prefix := strings.SplitN(stem, "-", 2)[0]
		title, rawStatus, v1ID := "", "", ""
		if h1 := regexp.MustCompile(`(?m)^# +(.+)$`).FindStringSubmatch(body); h1 != nil {
			title = strings.TrimSpace(h1[1])
		}
		if stl := statusRE.FindStringSubmatch(body); stl != nil {
			rawStatus = strings.TrimSpace(stl[1])
		}
		if fm != nil {
			if v, ok := fm["title"].(string); ok && strings.TrimSpace(v) != "" {
				title = strings.TrimSpace(v)
			}
			if v, ok := fm["status"].(string); ok && strings.TrimSpace(v) != "" {
				rawStatus = strings.TrimSpace(v)
			}
			if v, ok := fm["id"].(string); ok && v != "" {
				prefix = v
			}
		}
		if title == "" || rawStatus == "" {
			unparsed++
			r.plan(filepath.ToSlash(filepath.Join(project.LegacyDir, "tasks", e.Name())), b)
			continue
		}
		id := prefix
		if !idRE.MatchString(id) || used[strings.ToLower(id)] {
			v1ID, id = prefix, newID(used)
		}
		used[strings.ToLower(id)] = true
		title = stripIDPrefix(title, id, v1ID)
		title = textx.Truncate(title, 120)
		if len([]rune(title)) < 3 {
			title = "Task " + id
		}
		ts := commitTS(p, rel)
		legacy := map[string]any{"source": rel, "status": rawStatus}
		if v1ID != "" {
			legacy["v1_id"] = v1ID
		}
		if fm != nil {
			legacy["frontmatter"] = fm
		}
		m := task.Meta{ID: id, Title: title, Status: StatusMap(rawStatus), Profile: "standard", Created: ts, Updated: ts, Legacy: legacy}
		if err := schema.Validate("task", m); err != nil {
			unparsed++
			r.plan(filepath.ToSlash(filepath.Join(project.LegacyDir, "tasks", e.Name())), b)
			continue
		}
		out, _ := doc.Join(m, body)
		slug := project.Slug(strings.TrimPrefix(stem, prefix), 48)
		if slug == "untitled" {
			slug = project.Slug(title, 48)
		}
		r.plan(filepath.ToSlash(filepath.Join(project.TasksDir, id+"-"+slug+".md")), out)
		r.Counts["tasks"]++
	}
	if unparsed > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%d file(s) in docs/ai/tasks are not tasks: moved to docs/ai/_legacy/tasks/", unparsed))
	}

	// handoffs
	if b, err := os.ReadFile(p.Path(project.DocsAI, "handoff.md")); err == nil {
		rel := project.DocsAI + "/handoff.md"
		r.keep(p, rel)
		r.removals = append(r.removals, rel) // copied to _legacy; replaced by a generated index
		_, secs := splitSections(string(b))
		prev := commitTS(p, rel)
		seen := map[string]int{}
		for _, s := range secs {
			at := prev
			if m := dateRE.FindStringSubmatch(head40(s.head)); m != nil {
				at = isoTS(m[1], m[2])
			}
			prev = at
			stamp := strings.NewReplacer("-", "", ":", "").Replace(at)
			seen[stamp]++
			name := stamp + "-legacy"
			if seen[stamp] > 1 {
				name += fmt.Sprintf("-%d", seen[stamp])
			}
			m := map[string]any{"at": at, "by": "unknown", "outcome": "legacy"}
			if err := schema.Validate("handoff", m); err != nil {
				return nil, err
			}
			out, _ := doc.Join(m, s.head+s.body)
			r.plan(filepath.ToSlash(filepath.Join(project.HandoffDir, name+".md")), out)
			r.Counts["handoffs"]++
		}
	}

	// knowledge -> inbox
	if b, err := os.ReadFile(p.Path(project.DocsAI, "knowledge.md")); err == nil {
		rel := project.DocsAI + "/knowledge.md"
		r.keep(p, rel)
		r.removals = append(r.removals, rel) // copied to _legacy; replaced by a generated index
		fallback := commitTS(p, rel)[:10]
		var lines []string
		cur := fallback
		for _, chunk := range splitBullets(string(b)) {
			if chunk.heading != "" {
				if m := dateRE.FindStringSubmatch(chunk.heading); m != nil {
					cur = m[1]
				}
				continue
			}
			if len(strings.TrimSpace(chunk.text)) < 3 {
				continue
			}
			e := knowledge.Entry{Text: chunk.text, Date: cur, Topic: "general", Source: "migrated"}
			check := e
			check.Text = textx.Truncate(e.Text, 4000)
			if err := schema.Validate("knowledge-entry", check); err != nil {
				return nil, err
			}
			if len([]rune(e.Text)) > 4000 {
				r.Warnings = append(r.Warnings, "a knowledge entry exceeds 4000 characters; kept intact")
			}
			lines = append(lines, e.Render())
			r.Counts["knowledge"]++
		}
		r.plan(project.KnowDir+"/_inbox.md", []byte("# Knowledge inbox\n\n<!-- New entries land here; `aitk knowledge compact` files them by topic. -->\n\n"+strings.Join(lines, "\n")+"\n"))
	}

	// decisions -> ADRs
	if b, err := os.ReadFile(p.Path(project.DocsAI, "decisions.md")); err == nil {
		rel := project.DocsAI + "/decisions.md"
		r.keep(p, rel)
		r.removals = append(r.removals, rel) // copied to _legacy; replaced by a generated index
		_, secs := splitSections(string(b))
		n := 0
		existing, _ := os.ReadDir(p.Path(project.ADRDir))
		for _, e := range existing {
			if regexp.MustCompile(`^\d{4}-`).MatchString(e.Name()) {
				n++
			}
		}
		for _, s := range secs {
			n++
			title := strings.TrimSpace(strings.TrimPrefix(s.head, "##"))
			date := commitTS(p, rel)[:10]
			if m := dateRE.FindStringSubmatchIndex(title); m != nil && m[0] == 0 {
				date = title[m[2]:m[3]]
				title = strings.Trim(strings.TrimSpace(title[m[1]:]), "—- ")
			}
			if title == "" {
				title = "Decision"
			}
			out := fmt.Sprintf("---\nstatus: accepted\ndate: %s\n---\n\n# %s\n%s", date, title, s.body)
			r.plan(filepath.ToSlash(filepath.Join(project.ADRDir, fmt.Sprintf("%04d-%s.md", n, project.Slug(title, 60)))), []byte(out))
			r.Counts["adrs"]++
		}
	}

	// AGENTS.md
	if b, err := os.ReadFile(p.Path(project.AgentsFile)); err == nil {
		r.keep(p, project.AgentsFile)
		r.plan(project.AgentsFile, []byte(agentsmd.Apply(string(b))))
	} else {
		r.plan(project.AgentsFile, []byte(agentsmd.Apply("")))
	}

	for rel, b := range r.legacyBytes {
		if strings.HasPrefix(rel, project.DocsAI+"/") {
			r.V1Tokens += textx.Tokens(string(b))
		}
	}
	if dryRun {
		r.Written = sortedKeys(r.planned)
		r.Removed = r.removals
		r.Legacy = legacyNames(r.legacyBytes)
		return r, nil
	}
	if err := r.apply(p); err != nil {
		return r, err
	}
	q, err := project.Open(p.Root)
	if err != nil {
		return r, err
	}
	if res, err := knowledge.Compact(q, q.Config.ArchiveAfterDays(), false); err == nil {
		r.Counts["knowledge_topics"] = len(res.Topics)
		r.Counts["knowledge_archived"] = res.Archived
		r.Counts["knowledge_duplicates_merged"] = res.Duplicates
	}
	if err := handoff.WriteIndex(q); err != nil {
		return r, err
	}
	if _, err := compat.EnsureGitattributes(q, false); err != nil {
		return r, err
	}
	return r, compat.Write(q)
}

func (r *Report) plan(rel string, b []byte) { r.planned[rel] = b }

func (r *Report) keep(p *project.Project, rel string) {
	if b, err := os.ReadFile(p.Path(rel)); err == nil {
		r.legacyBytes[rel] = b
	}
}

func legacyName(rel string) string {
	if strings.HasPrefix(rel, project.DocsAI+"/") {
		return project.LegacyDir + "/" + strings.TrimPrefix(rel, project.DocsAI+"/")
	}
	return project.LegacyDir + "/" + rel
}

func legacyNames(m map[string][]byte) []string {
	var out []string
	for rel := range m {
		out = append(out, legacyName(rel))
	}
	sort.Strings(out)
	return out
}

// apply writes legacy copies first (verified byte for byte), then the new files.
func (r *Report) apply(p *project.Project) error {
	for rel, b := range r.legacyBytes {
		dst := legacyName(rel)
		if err := fsx.WriteFile(p.Path(dst), b, 0o644); err != nil {
			return err
		}
		if got, err := os.ReadFile(p.Path(dst)); err != nil || string(got) != string(b) {
			return fmt.Errorf("legacy copy of %s does not match; aborting before any change", rel)
		}
		r.Legacy = append(r.Legacy, dst)
	}
	sort.Strings(r.Legacy)
	for _, rel := range r.removals {
		if _, planned := r.planned[rel]; !planned {
			if err := os.Remove(p.Path(rel)); err == nil {
				r.Removed = append(r.Removed, rel)
			}
		}
	}
	for _, rel := range sortedKeys(r.planned) {
		if err := fsx.WriteFile(p.Path(rel), r.planned[rel], 0o644); err != nil {
			return err
		}
		r.Written = append(r.Written, rel)
	}
	for _, d := range []string{project.TasksDir, project.HandoffDir, project.KnowDir, project.ADRDir} {
		os.MkdirAll(p.Path(d), 0o755)
	}
	return r.writeReport(p)
}

func (r *Report) writeReport(p *project.Project) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Migration v1 → v2\n\nDate: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	keys := sortedKeys(toAny(r.Counts))
	for _, k := range keys {
		fmt.Fprintf(&b, "- %s: %d\n", k, r.Counts[k])
	}
	if len(r.Warnings) > 0 {
		b.WriteString("\n## Warnings\n")
		for _, w := range r.Warnings {
			b.WriteString("- " + w + "\n")
		}
	}
	b.WriteString("\n## Preserved verbatim\n")
	for _, l := range r.Legacy {
		b.WriteString("- " + l + "\n")
	}
	return fsx.WriteFile(p.Path(project.LegacyDir, "MIGRATION.md"), []byte(b.String()), 0o644)
}

// ---- helpers ----

type section struct{ head, body string }

func splitSections(text string) (string, []section) {
	lines := strings.SplitAfter(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var pre strings.Builder
	var secs []section
	inFence := false
	for _, l := range lines {
		if strings.HasPrefix(l, "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(l, "## ") {
			secs = append(secs, section{head: l})
			continue
		}
		if len(secs) == 0 {
			pre.WriteString(l)
		} else {
			secs[len(secs)-1].body += l
		}
	}
	return pre.String(), secs
}

type chunk struct{ heading, text string }

// splitBullets yields headings and top-level bullets (with indented continuation lines).
func splitBullets(text string) []chunk {
	var out []chunk
	var cur *chunk
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(l, "#"):
			flush()
			out = append(out, chunk{heading: l})
		case strings.HasPrefix(l, "- "):
			flush()
			cur = &chunk{text: l[2:]}
		case cur != nil && strings.HasPrefix(l, "  ") && strings.TrimSpace(l) != "":
			cur.text += "\n" + strings.TrimPrefix(l, "  ")
		default:
			flush()
		}
	}
	flush()
	return out
}

func head40(s string) string {
	r := []rune(s)
	if len(r) > 40 {
		r = r[:40]
	}
	return string(r)
}

func isoTS(date, clock string) string {
	if clock == "" {
		clock = "00:00:00"
	}
	if len(clock) == 5 {
		clock += ":00"
	}
	return date + "T" + clock + "Z"
}

var tsRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)

func matchTS(text, key string) string {
	m := regexp.MustCompile(`(?m)^` + key + `: *(\S+)$`).FindStringSubmatch(text)
	if m != nil && tsRE.MatchString(m[1]) {
		return m[1]
	}
	return ""
}

func commitTS(p *project.Project, rel string) string {
	if d := gitx.LastCommitDate(p.Root, rel); d != "" {
		if t, err := time.Parse(time.RFC3339, d); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05Z")
		}
	}
	if info, err := os.Stat(p.Path(rel)); err == nil {
		return info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
	}
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

func stripHeader(text string) string {
	var keep []string
	for _, l := range strings.SplitAfter(text, "\n") {
		if regexp.MustCompile(`^(# Current Task|Status:|Task ID:|Started At:|Updated At:|Project Root:)`).MatchString(l) {
			continue
		}
		keep = append(keep, l)
	}
	return strings.TrimLeft(strings.Join(keep, ""), "\n")
}

const crockford = "0123456789abcdefghjkmnpqrstvwxyz"

func newID(used map[string]bool) string {
	var seed int64 = time.Now().UnixNano()
	for n := 4; ; n++ {
		for i := 0; i < 16; i++ {
			buf := make([]byte, n)
			for j := range buf {
				seed = seed*6364136223846793005 + 1442695040888963407
				buf[j] = crockford[uint64(seed)>>59]
			}
			id := "T-" + string(buf)
			if !used[strings.ToLower(id)] {
				used[strings.ToLower(id)] = true
				return id
			}
		}
	}
}

func renderConfig(c project.Config) []byte {
	var b strings.Builder
	b.WriteString("# aitk project configuration (migrated from .ai-toolkit/project.env)\n\n")
	enc := toml.NewEncoder(&b)
	enc.Indent = ""
	enc.Encode(c)
	return []byte(b.String())
}

// stripIDPrefix removes a leading "<id>: " / "<id> — " that duplicates the task ID.
func stripIDPrefix(title string, ids ...string) string {
	for _, id := range ids {
		if id == "" {
			continue
		}
		rest, ok := strings.CutPrefix(title, id)
		if !ok {
			continue
		}
		trimmed := strings.TrimLeft(rest, " ")
		if !strings.HasPrefix(trimmed, ":") && !strings.HasPrefix(trimmed, "—") && !strings.HasPrefix(trimmed, "–") && !strings.HasPrefix(trimmed, "- ") {
			continue // e.g. "F5f pass-21 — …": the ID is part of the title
		}
		if rest = strings.TrimLeft(trimmed, " :—–-"); len([]rune(rest)) >= 3 {
			return rest
		}
	}
	return title
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func toAny(m map[string]int) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	return out
}
