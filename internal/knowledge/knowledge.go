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

	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
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

// block is one entry and its line range [start, end) in a file.
type block struct {
	start, end int
	entry      Entry
}

// parseBlocks finds entries: a line starting with "- " plus following lines indented by two
// spaces (an indented line with nothing else is a blank line inside the entry). Everything
// else in the file (headings, comments, prose) is not an entry and is never touched.
func parseBlocks(lines []string, topic, file, fallbackDate string) []block {
	var out []block
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "- ") {
			continue
		}
		j := i + 1
		for j < len(lines) && strings.HasPrefix(lines[j], "  ") {
			j++
		}
		// Trailing whitespace-only continuation lines belong to the gap, not the entry.
		for j > i+1 && strings.TrimSpace(lines[j-1]) == "" {
			j--
		}
		var b strings.Builder
		b.WriteString(lines[i][2:])
		for _, l := range lines[i+1 : j] {
			b.WriteString("\n" + strings.TrimPrefix(l, "  "))
		}
		e := Entry{Text: b.String(), Date: fallbackDate, Topic: topic, File: file}
		finish(&e)
		out = append(out, block{start: i, end: j, entry: e})
		i = j - 1
	}
	return out
}

func splitLines(content string) []string {
	return strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
}

// Parse extracts entries from one knowledge file.
func Parse(content, topic, file, fallbackDate string) []Entry {
	var out []Entry
	for _, b := range parseBlocks(splitLines(content), topic, file, fallbackDate) {
		out = append(out, b.entry)
	}
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
}

type kfile struct {
	name          string // e.g. general.md
	topic         string
	lines         []string
	blocks        []block
	remove        map[int]bool   // block indexes to remove
	repl          map[int]string // block index -> replacement rendering
	appendEntries []Entry
	exists        bool
}

func (f *kfile) render() string {
	var out []string
	bi := 0
	for i := 0; i < len(f.lines); {
		if bi < len(f.blocks) && f.blocks[bi].start == i {
			b := f.blocks[bi]
			switch {
			case f.remove[bi]:
			case f.repl[bi] != "":
				out = append(out, strings.Split(f.repl[bi], "\n")...)
			default:
				out = append(out, f.lines[b.start:b.end]...)
			}
			i = b.end
			bi++
			continue
		}
		out = append(out, f.lines[i])
		i++
	}
	s := strings.Join(out, "\n")
	if len(f.appendEntries) > 0 {
		s = strings.TrimRight(s, "\n") + "\n"
		if strings.TrimSpace(s) == "" {
			s = "# Knowledge: " + f.topic + "\n\n"
		}
		for _, e := range f.appendEntries {
			s += e.Render() + "\n"
		}
	}
	return s
}

// Compact files inbox entries by topic, drops exact (normalized) duplicates arriving in the
// inbox, and archives old unpinned entries (docs/spec/workflow.md §8). It edits entry blocks
// only: headings, comments, and prose in any file are kept byte for byte, and no entry text
// is deleted (a duplicate is only dropped when the same text already exists).
func Compact(p *project.Project, archiveAfterDays int, dryRun bool) (*CompactResult, error) {
	res := &CompactResult{Topics: map[string]int{}}
	cutoff := time.Now().UTC().AddDate(0, 0, -archiveAfterDays).Format("2006-01-02")
	dir := p.Path(project.KnowDir)
	entries, _ := os.ReadDir(dir)
	files := map[string]*kfile{}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(project.KnowDir, e.Name()))
		b, err := os.ReadFile(p.Path(rel))
		if err != nil {
			continue
		}
		topic := strings.TrimSuffix(e.Name(), ".md")
		lines := splitLines(string(b))
		files[topic] = &kfile{name: e.Name(), topic: topic, lines: lines, exists: true,
			blocks: parseBlocks(lines, topic, rel, fileDate(p, rel)), remove: map[int]bool{}, repl: map[int]string{}}
		names = append(names, topic)
	}
	sort.Strings(names)
	type ref struct {
		topic    string
		idx      int // block index, or index into appendEntries when appended
		appended bool
	}
	seen := map[string]ref{}
	archive := map[string][]Entry{}
	file := func(topic string) *kfile {
		if f, ok := files[topic]; ok {
			return f
		}
		f := &kfile{name: topic + ".md", topic: topic, remove: map[int]bool{}, repl: map[int]string{}}
		files[topic] = f
		return f
	}
	// Topic files first (existing text wins duplicate checks), then the inbox.
	order := append([]string{}, names...)
	sort.SliceStable(order, func(i, j int) bool { return order[i] != inbox && order[j] == inbox })
	for _, topic := range order {
		f := files[topic]
		for i, b := range f.blocks {
			e := b.entry
			key := Normalize(e.Text)
			if topic == inbox {
				if r, dup := seen[key]; dup && key != "" {
					// Same text already recorded: merge tags/pin into it and drop the copy.
					first := files[r.topic]
					if r.appended {
						fe := &first.appendEntries[r.idx]
						fe.Tags, fe.Pinned = union(fe.Tags, e.Tags), fe.Pinned || e.Pinned
					} else {
						fe := first.blocks[r.idx].entry
						merged := fe
						merged.Tags, merged.Pinned = union(fe.Tags, e.Tags), fe.Pinned || e.Pinned
						if strings.Join(merged.Tags, ",") != strings.Join(fe.Tags, ",") || merged.Pinned != fe.Pinned {
							first.repl[r.idx] = merged.Render()
							first.blocks[r.idx].entry = merged
						}
					}
					f.remove[i] = true
					res.Duplicates++
					continue
				}
				f.remove[i] = true
				res.Moved++
				if !e.Pinned && e.Date < cutoff {
					archive[quarter(e.Date)] = append(archive[quarter(e.Date)], e)
					res.Archived++
					continue
				}
				t := file(topicFor(e))
				t.appendEntries = append(t.appendEntries, e)
				if key != "" {
					seen[key] = ref{t.topic, len(t.appendEntries) - 1, true}
				}
				continue
			}
			if !e.Pinned && e.Date < cutoff {
				f.remove[i] = true
				archive[quarter(e.Date)] = append(archive[quarter(e.Date)], e)
				res.Archived++
				continue
			}
			if _, dup := seen[key]; !dup && key != "" {
				seen[key] = ref{topic, i, false}
			}
		}
	}
	for _, f := range files {
		n := len(f.appendEntries)
		for i := range f.blocks {
			if !f.remove[i] {
				n++
			}
		}
		if f.topic != inbox && n > 0 {
			res.Topics[f.topic] = n
		}
	}
	if dryRun {
		return res, nil
	}
	for _, f := range files {
		changed := len(f.remove) > 0 || len(f.repl) > 0 || len(f.appendEntries) > 0
		if !changed {
			continue
		}
		if err := fsx.WriteFile(filepath.Join(dir, f.name), []byte(f.render()), 0o644); err != nil {
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
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		for _, e := range es {
			content += e.Render() + "\n"
		}
		if err := fsx.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, err
		}
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

// Pin sets the pin flag on entries whose text contains query (case-insensitive), editing
// only those entry blocks.
func Pin(p *project.Project, query string) (int, error) {
	q := strings.ToLower(query)
	files, _ := os.ReadDir(p.Path(project.KnowDir))
	n := 0
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(project.KnowDir, f.Name()))
		b, _ := os.ReadFile(p.Path(rel))
		lines := splitLines(string(b))
		kf := &kfile{name: f.Name(), lines: lines, blocks: parseBlocks(lines, "", rel, fileDate(p, rel)), remove: map[int]bool{}, repl: map[int]string{}}
		for i, bl := range kf.blocks {
			if !bl.entry.Pinned && strings.Contains(strings.ToLower(bl.entry.Text), q) {
				e := bl.entry
				e.Pinned = true
				kf.repl[i] = e.Render()
				n++
			}
		}
		if len(kf.repl) > 0 {
			if err := fsx.WriteFile(p.Path(rel), []byte(kf.render()), 0o644); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}
