// Package knowledge implements docs/spec/workflow.md §8.
package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/innaka-tech/ai-toolkit/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/internal/project"
)

// Entry is one knowledge bullet (schemas/knowledge-entry.schema.json).
type Entry struct {
	Text   string   `json:"text"`
	Date   string   `json:"date"`
	Topic  string   `json:"topic"`
	Task   string   `json:"task,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Pinned bool     `json:"pinned,omitempty"`
	Source string   `json:"source,omitempty"`
	File   string   `json:"file,omitempty"`
}

const inbox = "_inbox"

var metaRe = regexp.MustCompile(`\s*<!-- aitk:k ([^>]*)-->\s*$`)

// Render formats an entry as one Markdown bullet (continuation lines indented).
func (e Entry) Render() string {
	var meta []string
	meta = append(meta, "date="+e.Date)
	if e.Task != "" {
		meta = append(meta, "task="+e.Task)
	}
	if len(e.Tags) > 0 {
		meta = append(meta, "tags="+strings.Join(e.Tags, ","))
	}
	if e.Source != "" && e.Source != "close" {
		meta = append(meta, "source="+e.Source)
	}
	if e.Pinned {
		meta = append(meta, "pin")
	}
	text := strings.ReplaceAll(strings.TrimSpace(e.Text), "\n", "\n  ")
	return fmt.Sprintf("- %s <!-- aitk:k %s -->", text, strings.Join(meta, " "))
}

// Parse extracts entries from one knowledge file.
func Parse(content, topic, file, fallbackDate string) []Entry {
	var out []Entry
	var cur *Entry
	flush := func() {
		if cur != nil {
			finish(cur)
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, l := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(l, "- "):
			flush()
			cur = &Entry{Text: l[2:], Date: fallbackDate, Topic: topic, File: file}
		case cur != nil && len(l) > 2 && strings.HasPrefix(l, "  ") && strings.TrimSpace(l) != "":
			cur.Text += "\n" + strings.TrimPrefix(l, "  ")
		default:
			flush()
		}
	}
	flush()
	return out
}

func finish(e *Entry) {
	m := metaRe.FindStringSubmatchIndex(e.Text)
	if m == nil {
		return
	}
	fields := strings.Fields(e.Text[m[2]:m[3]])
	e.Text = strings.TrimSpace(e.Text[:m[0]])
	for _, f := range fields {
		k, v, _ := strings.Cut(f, "=")
		switch k {
		case "date":
			e.Date = v
		case "task":
			e.Task = v
		case "tags":
			e.Tags = strings.Split(v, ",")
		case "source":
			e.Source = v
		case "pin":
			e.Pinned = true
		}
	}
}

// LoadAll reads every knowledge file (topics and inbox; not the archive).
func LoadAll(p *project.Project) []Entry {
	dir := p.Path(project.KnowDir)
	files, _ := os.ReadDir(dir)
	var out []Entry
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(project.KnowDir, f.Name()))
		b, err := os.ReadFile(p.Path(rel))
		if err != nil {
			continue
		}
		topic := strings.TrimSuffix(f.Name(), ".md")
		date := fileDate(p, rel)
		out = append(out, Parse(string(b), topic, rel, date)...)
	}
	return out
}

func fileDate(p *project.Project, rel string) string {
	if d := gitx.LastCommitDate(p.Root, rel); len(d) >= 10 {
		return d[:10]
	}
	if info, err := os.Stat(p.Path(rel)); err == nil {
		return info.ModTime().UTC().Format("2006-01-02")
	}
	return time.Now().UTC().Format("2006-01-02")
}

// Add appends an entry to the inbox.
func Add(p *project.Project, e Entry) error {
	path := p.Path(project.KnowDir, inbox+".md")
	b, _ := os.ReadFile(path)
	content := string(b)
	if content == "" {
		content = inboxHeader
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += e.Render() + "\n"
	return fsx.WriteFile(path, []byte(content), 0o644)
}

// InboxCount returns the number of inbox entries.
func InboxCount(p *project.Project) int {
	b, _ := os.ReadFile(p.Path(project.KnowDir, inbox+".md"))
	return len(Parse(string(b), inbox, "", ""))
}

// Normalize lowercases and strips punctuation and whitespace for duplicate detection.
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CompactResult reports what compaction did.
type CompactResult struct {
	Moved      int            `json:"moved"`
	Duplicates int            `json:"duplicates"`
	Archived   int            `json:"archived"`
	Topics     map[string]int `json:"topics"`
	Frozen     []string       `json:"frozen,omitempty"` // files with hand-written prose, left untouched
}

type kfile struct {
	name    string // e.g. general.md
	topic   string
	entries []Entry
	pure    bool // only headings, comments, blank lines, and entries
}

func loadFiles(p *project.Project) []kfile {
	dir := p.Path(project.KnowDir)
	files, _ := os.ReadDir(dir)
	var out []kfile
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(project.KnowDir, f.Name()))
		b, err := os.ReadFile(p.Path(rel))
		if err != nil {
			continue
		}
		topic := strings.TrimSuffix(f.Name(), ".md")
		out = append(out, kfile{name: f.Name(), topic: topic, entries: Parse(string(b), topic, rel, fileDate(p, rel)), pure: isPure(string(b))})
	}
	return out
}

// isPure reports whether content has nothing but headings, HTML comments, blank lines, and entries.
func isPure(content string) bool {
	inEntry, inComment := false, false
	for _, l := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		tl := strings.TrimSpace(l)
		switch {
		case inComment:
			if strings.Contains(l, "-->") {
				inComment = false
			}
		case strings.HasPrefix(l, "- "):
			inEntry = true
		case inEntry && strings.HasPrefix(l, "  ") && tl != "":
		case tl == "" || strings.HasPrefix(tl, "#"):
			inEntry = false
		case strings.HasPrefix(tl, "<!--"):
			inEntry = false
			inComment = !strings.Contains(tl, "-->")
		default:
			return false
		}
	}
	return true
}

// Compact runs the deterministic compaction (docs/spec/workflow.md §8). It never deletes
// text: exact duplicates are merged into the first occurrence, and files containing
// hand-written prose are left untouched.
func Compact(p *project.Project, archiveAfterDays int, dryRun bool) (*CompactResult, error) {
	res := &CompactResult{Topics: map[string]int{}}
	cutoff := time.Now().UTC().AddDate(0, 0, -archiveAfterDays).Format("2006-01-02")
	files := loadFiles(p)

	type ref struct {
		topic string
		idx   int
	}
	topics := map[string][]Entry{}
	archive := map[string][]Entry{}
	seen := map[string]*ref{}
	frozenKeys := map[string]bool{}
	rewrite := map[string]bool{} // topic files compaction owns

	// Frozen files first, so their text wins duplicate checks.
	for _, f := range files {
		if f.topic != inbox && !f.pure {
			res.Frozen = append(res.Frozen, f.name)
			for _, e := range f.entries {
				frozenKeys[Normalize(e.Text)] = true
			}
		}
	}
	// Pure topic files before the inbox, so existing entries keep precedence.
	sort.SliceStable(files, func(i, j int) bool { return files[i].topic != inbox && files[j].topic == inbox })
	for _, f := range files {
		if f.topic != inbox && !f.pure {
			continue
		}
		if f.topic != inbox {
			rewrite[f.topic] = true
		}
		for _, e := range f.entries {
			topic := f.topic
			if topic == inbox {
				topic = topicFor(e)
				res.Moved++
			}
			key := Normalize(e.Text)
			if key != "" && frozenKeys[key] {
				res.Duplicates++
				continue
			}
			if r, dup := seen[key]; dup && key != "" {
				first := &topics[r.topic][r.idx]
				first.Tags = union(first.Tags, e.Tags)
				first.Pinned = first.Pinned || e.Pinned
				res.Duplicates++
				continue
			}
			if !e.Pinned && e.Date < cutoff {
				q := quarter(e.Date)
				archive[q] = append(archive[q], e)
				res.Archived++
				continue
			}
			topics[topic] = append(topics[topic], e)
			seen[key] = &ref{topic, len(topics[topic]) - 1}
			rewrite[topic] = true
		}
	}
	for t, es := range topics {
		res.Topics[t] = len(es)
	}
	if dryRun {
		return res, nil
	}
	dir := p.Path(project.KnowDir)
	for t := range rewrite {
		es := topics[t]
		path := filepath.Join(dir, t+".md")
		if len(es) == 0 {
			os.Remove(path)
			continue
		}
		sort.SliceStable(es, func(i, j int) bool {
			if es[i].Pinned != es[j].Pinned {
				return es[i].Pinned
			}
			return es[i].Date > es[j].Date
		})
		var sb strings.Builder
		fmt.Fprintf(&sb, "# Knowledge: %s\n\n", t)
		for _, e := range es {
			sb.WriteString(e.Render() + "\n")
		}
		if err := fsx.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			return nil, err
		}
	}
	for q, es := range archive {
		path := filepath.Join(dir, "_archive", q+".md")
		b, _ := os.ReadFile(path)
		content := string(b)
		if content == "" {
			content = "# Knowledge archive " + q + "\n\n"
		}
		for _, e := range es {
			content += e.Render() + "\n"
		}
		if err := fsx.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, err
		}
	}
	if err := fsx.WriteFile(filepath.Join(dir, inbox+".md"), []byte(inboxHeader), 0o644); err != nil {
		return nil, err
	}
	return res, nil
}

const inboxHeader = "# Knowledge inbox\n\n<!-- New entries land here; `aitk knowledge compact` files them by topic. -->\n\n"

func topicFor(e Entry) string {
	if len(e.Tags) > 0 && e.Tags[0] != "" {
		return project.Slug(e.Tags[0], 62)
	}
	return "general"
}

func quarter(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "undated"
	}
	return fmt.Sprintf("%d-Q%d", t.Year(), (int(t.Month())-1)/3+1)
}

func union(a, b []string) []string {
	set := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if s != "" && !set[s] {
			set[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Scored is a search hit.
type Scored struct {
	Entry
	Score float64 `json:"score"`
}

// Search ranks entries by term overlap, pin, tag/topic match, and recency.
func Search(entries []Entry, query string, tags []string, limit int) []Scored {
	terms := words(query)
	tagset := map[string]bool{}
	for _, t := range tags {
		tagset[strings.ToLower(t)] = true
	}
	ref := ""
	for _, e := range entries {
		if e.Date > ref {
			ref = e.Date
		}
	}
	var out []Scored
	for _, e := range entries {
		s := 0.0
		ew := words(e.Text + " " + strings.Join(e.Tags, " ") + " " + e.Topic)
		for t := range terms {
			if ew[t] {
				s += 1
			}
		}
		for _, t := range e.Tags {
			if tagset[strings.ToLower(t)] {
				s += 2
			}
		}
		if tagset[e.Topic] {
			s += 2
		}
		if len(terms) > 0 && len(tagset) == 0 && s == 0 && !e.Pinned {
			continue
		}
		if e.Pinned {
			s += 3
		}
		s += recency(e.Date, ref)
		out = append(out, Scored{Entry: e, Score: s})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Date > out[j].Date
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// recency is relative to the newest entry (not the clock), so the ranking is
// deterministic for identical repository state.
func recency(date, ref string) float64 {
	d, err1 := time.Parse("2006-01-02", date)
	r, err2 := time.Parse("2006-01-02", ref)
	if err1 != nil || err2 != nil {
		return 0
	}
	days := r.Sub(d).Hours() / 24
	if days < 0 {
		days = 0
	}
	return 1 / (1 + days/30) // 1.0 for the newest, 0.5 a month older
}

var wordRe = regexp.MustCompile(`[\p{L}\p{N}_]{3,}`)

func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(s), -1) {
		out[w] = true
	}
	return out
}

// Pin sets the pin flag on entries whose text contains query (case-insensitive).
func Pin(p *project.Project, query string) (int, error) {
	q := strings.ToLower(query)
	files, _ := os.ReadDir(p.Path(project.KnowDir))
	n := 0
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		path := p.Path(project.KnowDir, f.Name())
		b, _ := os.ReadFile(path)
		lines := strings.Split(string(b), "\n")
		var out []string
		changed := false
		for i := 0; i < len(lines); i++ {
			if !strings.HasPrefix(lines[i], "- ") {
				out = append(out, lines[i])
				continue
			}
			j := i + 1
			for j < len(lines) && strings.HasPrefix(lines[j], "  ") && strings.TrimSpace(lines[j]) != "" {
				j++
			}
			block := strings.Join(lines[i:j], "\n")
			es := Parse(block, "", "", fileDate(p, filepath.ToSlash(filepath.Join(project.KnowDir, f.Name()))))
			if len(es) == 1 && !es[0].Pinned && strings.Contains(strings.ToLower(es[0].Text), q) {
				es[0].Pinned = true
				out = append(out, strings.Split(es[0].Render(), "\n")...)
				changed = true
				n++
			} else {
				out = append(out, lines[i:j]...)
			}
			i = j - 1
		}
		if changed {
			if err := fsx.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}
