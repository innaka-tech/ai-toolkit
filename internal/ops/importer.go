package ops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
	"github.com/innaka-tech/ai-toolkit/v2/internal/textx"
)

// ImportResult reports imported and skipped items.
type ImportResult struct {
	Source   string   `json:"source"`
	Imported []string `json:"imported"`
	Skipped  []string `json:"skipped"` // already present
	DryRun   bool     `json:"dry_run"`
	Synced   []string `json:"synced,omitempty"`                 // done in aitk, now checked in the source file
	Drift    []string `json:"checked_in_source_only,omitempty"` // checked in the source file, not done in aitk
}

var checkbox = regexp.MustCompile(`^\s*[-*] \[( |x|X)\] (.+)$`)

type importItem struct {
	id, title, objective string
	done                 bool
	status, profile      string // optional: explicit status; pinned profile
	criteria, tags       []string
	doneAC               []int // 1-based criteria already checked in the source
	dependsOn            []string
	legacy               map[string]any
}

func importItems(p *project.Project, source string, items []importItem, dry bool) (*ImportResult, error) {
	res := &ImportResult{Source: source, Imported: []string{}, Skipped: []string{}, DryRun: dry}
	err := WithLock(p, func() error {
		all, _ := task.List(p)
		byItem := map[string]*task.Task{} // tasks imported earlier, by source file and item text
		ids := map[string]bool{}
		for _, t := range all {
			ids[strings.ToLower(t.ID)] = true
			f, _ := t.Legacy["file"].(string)
			if item, _ := t.Legacy["item"].(string); f != "" && item != "" {
				byItem[f+"\x00"+item] = t
			}
		}
		batch := map[string]bool{}
		for _, it := range items {
			file, _ := it.legacy["file"].(string)
			item, _ := it.legacy["item"].(string)
			if prev := byItem[file+"\x00"+item]; file != "" && item != "" && prev != nil {
				it.id = prev.ID // the same item, wherever it moved in the file
			} else if t, err := task.Find(p, it.id); err == nil && file != "" && item != "" {
				if other, _ := t.Legacy["item"].(string); other != "" && other != item {
					it.id = freeID(it.id, ids, batch) // a new item at a position another item had
				}
			}
			if batch[strings.ToLower(it.id)] { // two items with one ID in this import
				it.id = freeID(it.id, ids, batch)
			}
			batch[strings.ToLower(it.id)] = true
			if t, err := task.Find(p, it.id); err == nil {
				res.Skipped = append(res.Skipped, it.id)
				switch {
				case t.Status == task.Done && !it.done && !dry:
					if ok, _ := syncSource(p, t); ok {
						res.Synced = append(res.Synced, t.ID)
					}
				case t.Status == task.Done && !it.done:
					res.Synced = append(res.Synced, t.ID)
				case it.done && t.Status != task.Done && t.Status != task.Cancelled:
					res.Drift = append(res.Drift, t.ID)
				}
				if len(t.DependsOn) == 0 && len(it.dependsOn) > 0 && !dry { // imported before dependencies existed
					t.DependsOn, t.Updated = it.dependsOn, task.Now()
					if err := task.Save(p, t); err != nil {
						return err
					}
				}
				continue
			}
			res.Imported = append(res.Imported, it.id)
			if dry {
				continue
			}
			title := textx.Truncate(strings.TrimSpace(it.title), 120)
			if len([]rune(title)) < 3 {
				title = "Task " + it.id
			}
			now := task.Now()
			st := task.Todo
			if it.done {
				st = task.Done
			}
			if it.status != "" {
				st = it.status
			}
			prof, pinned := "standard", false
			if it.profile != "" {
				prof, pinned = it.profile, true
			}
			t := &task.Task{Meta: task.Meta{ID: it.id, Title: title, Status: st, Profile: prof, ProfilePinned: pinned, Tags: slugs(it.tags),
				Created: now, Updated: now, CreatedBy: Tool(), Legacy: it.legacy, DependsOn: it.dependsOn}, Body: task.NewBody(it.objective, it.criteria)}
			if len(it.doneAC) > 0 {
				t.MarkCriteria(it.doneAC)
			}
			if st == task.Done {
				t.Closed = now
			}
			if err := task.Save(p, t); err != nil {
				return fmt.Errorf("%s: %w", it.id, err)
			}
		}
		return nil
	})
	return res, err
}

// freeID returns id with the first free "-N" suffix.
func freeID(id string, taken ...map[string]bool) string {
	for n := 2; ; n++ {
		c := fmt.Sprintf("%s-%d", id, n)
		if len(c) > 32 {
			c = fmt.Sprintf("%s-%d", strings.TrimRight(id[:32-len(fmt.Sprint(n))-1], "-"), n)
		}
		free := true
		for _, m := range taken {
			free = free && !m[strings.ToLower(c)]
		}
		if free {
			return c
		}
	}
}

// ImportGitHub imports issues through the GitHub CLI (`gh`). Checkbox lines in the issue body
// become acceptance criteria; labels become tags; closed issues become done tasks.
func ImportGitHub(p *project.Project, repo, label, state string, limit int, dry bool) (*ImportResult, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, apperr.New("E_USAGE", apperr.ExitUsage, "install the GitHub CLI: https://cli.github.com", "gh is not installed")
	}
	args := []string{"issue", "list", "--json", "number,title,body,labels,state,url", "--limit", fmt.Sprint(limit), "--state", state}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	if label != "" {
		args = append(args, "--label", label)
	}
	cmd := exec.Command("gh", args...)
	cmd.Dir = p.Root
	out, err := cmd.Output()
	if err != nil {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			msg = strings.TrimSpace(string(ee.Stderr))
		}
		return nil, apperr.New("E_IMPORT", apperr.ExitRuntime, "check --repo, then gh auth status", "gh issue list failed: %s", msg)
	}
	var issues []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		URL    string `json:"url"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, err
	}
	var items []importItem
	for _, is := range issues {
		it := importItem{id: fmt.Sprintf("GH-%d", is.Number), title: is.Title, done: strings.EqualFold(is.State, "closed"),
			legacy: map[string]any{"source": "github", "url": is.URL}}
		var rest []string
		fence := false
		for _, l := range strings.Split(strings.ReplaceAll(is.Body, "\r\n", "\n"), "\n") {
			if t := strings.TrimSpace(l); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
				fence = !fence
			}
			if m := checkbox.FindStringSubmatch(l); m != nil && !fence && !strings.HasPrefix(l, "    ") && !strings.HasPrefix(l, "\t") {
				it.criteria = append(it.criteria, strings.TrimSpace(m[2]))
				if m[1] != " " {
					it.doneAC = append(it.doneAC, len(it.criteria))
				}
			} else {
				rest = append(rest, l)
			}
		}
		it.objective = textx.Truncate(strings.TrimSpace(strings.Join(rest, "\n")), 2000)
		for _, l := range is.Labels {
			it.tags = append(it.tags, l.Name)
		}
		items = append(items, it)
	}
	return importItems(p, "github-issues", items, dry)
}

var taskIDToken = regexp.MustCompile(`^(T\d{3,})\b\s*((?:\[[^\]]*\]\s*)*)(.*)$`)
var marker = regexp.MustCompile(`\[([^\]]*)\]`)

// explicitDeps finds "depends on T012, T013" (and "after T005") in a Spec Kit description.
var explicitDeps = regexp.MustCompile(`(?i)(?:depends on|after|requires)\s+((?:T\d{3,}(?:\s*(?:,|and|&)\s*)?)+)`)
var specTaskID = regexp.MustCompile(`T\d{3,}`)

// ImportChecklist imports Spec Kit (specs/*/tasks.md) or OpenSpec (openspec/changes/*/tasks.md)
// checklists: each checkbox line becomes a task; "T001 …" keeps its ID (prefixed by the spec name).
// Spec Kit markers become tags ([P] → parallel, [US1] → us1) and its phases become dependencies:
// a task waits for the previous sequential task, and a sequential task waits for the parallel
// tasks before it. Tasks link the feature's spec.md and plan.md. Importing again checks off the
// items of tasks done in aitk and reports items checked only in the file.
func ImportChecklist(p *project.Project, kind string, paths []string, dry bool) (*ImportResult, error) {
	if len(paths) == 0 && kind == "markdown" {
		return nil, apperr.Usage("give the markdown file(s) to import")
	}
	if len(paths) == 0 {
		pattern := map[string]string{"spec-kit": "specs/*/tasks.md", "openspec": "openspec/changes/*/tasks.md"}[kind]
		paths, _ = filepath.Glob(p.Path(filepath.FromSlash(pattern)))
		if len(paths) == 0 {
			return nil, apperr.New("E_IMPORT", apperr.ExitUsage, "aitk import "+kind+" <path/to/tasks.md>", "no %s found", pattern)
		}
	}
	var items []importItem
	batchPrefixes := map[string]string{} // prefix → file, for features imported together
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, apperr.New("E_IMPORT", apperr.ExitUsage, "check the path", "%v", err)
		}
		// Spec Kit feature dirs look like "001-checkout": drop the numeric prefix for readable IDs.
		base := filepath.Base(filepath.Dir(path))
		if kind == "markdown" {
			base = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		feature := strings.TrimLeft(project.Slug(base, 30), "0123456789-")
		if feature == "" {
			feature = "spec"
		}
		rel, _ := filepath.Rel(p.Root, path)
		var context []string
		if kind == "spec-kit" {
			for _, f := range []string{"spec.md", "plan.md", "data-model.md", "research.md", "quickstart.md"} {
				if _, err := os.Stat(filepath.Join(filepath.Dir(path), f)); err == nil {
					context = append(context, filepath.ToSlash(filepath.Join(filepath.Dir(rel), f)))
				}
			}
		}
		n := 0
		prefix := checklistPrefix(p, feature, filepath.ToSlash(rel))
		if owner, ok := batchPrefixes[prefix]; ok && owner != filepath.ToSlash(rel) {
			sum := sha256.Sum256([]byte(filepath.ToSlash(rel)))
			prefix = strings.ToUpper(feature[:min(len(feature), 7)]) + "-" + strings.ToUpper(hex.EncodeToString(sum[:])[:4])
		}
		batchPrefixes[prefix] = filepath.ToSlash(rel)
		barriers, group := []string(nil), []string(nil)
		lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
		fence := false
		for _, l := range lines {
			if t := strings.TrimSpace(l); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
				fence = !fence
				continue
			}
			if fence {
				continue
			}
			if kind == "spec-kit" && strings.HasPrefix(strings.TrimSpace(l), "## ") && len(group) > 0 {
				// A new phase waits for the whole parallel group that ended the previous one.
				barriers, group = group, nil
				continue
			}
			m := checkbox.FindStringSubmatch(l)
			if m == nil || strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") {
				continue // not a top-level item: nested sub-steps belong to their parent
			}
			n++
			item := strings.TrimSpace(m[2])
			text := item
			id := fmt.Sprintf("%s-%d", prefix, n)
			ref := ""
			var tags []string
			parallel := false
			if tm := taskIDToken.FindStringSubmatch(text); tm != nil {
				ref = tm[1]
				id = prefix + "-" + tm[1]
				text = strings.TrimSpace(tm[3])
				for _, mk := range marker.FindAllStringSubmatch(tm[2], -1) {
					switch v := strings.ToLower(strings.TrimSpace(mk[1])); v {
					case "p":
						parallel = true
						tags = append(tags, "parallel")
					case "":
					default:
						tags = append(tags, v)
					}
				}
			}
			if text == "" {
				text = item
			}
			id = strings.Trim(regexp.MustCompile(`[^A-Za-z0-9-]+`).ReplaceAllString(id, "-"), "-")
			if !task.IDPattern.MatchString(id) {
				id = "S-" + id
			}
			it := importItem{id: id, title: text, done: m[1] != " ", objective: text, tags: tags,
				legacy: map[string]any{"source": kind, "file": filepath.ToSlash(rel), "item": item}}
			if ref != "" {
				it.legacy["ref"] = ref
			}
			if len(context) > 0 {
				it.objective += "\n\nContext: " + strings.Join(context, ", ")
			}
			if kind == "spec-kit" {
				for _, m := range explicitDeps.FindAllStringSubmatch(text, -1) {
					for _, dep := range specTaskID.FindAllString(m[1], -1) {
						if d := prefix + "-" + dep; d != id && !contains(it.dependsOn, d) {
							it.dependsOn = append(it.dependsOn, d)
						}
					}
				}
				wait := barriers
				if !parallel && len(group) > 0 {
					wait = group
				}
				for _, d := range wait {
					if d != id && !contains(it.dependsOn, d) {
						it.dependsOn = append(it.dependsOn, d)
					}
				}
				if parallel {
					group = append(group, id)
				} else {
					barriers, group = []string{id}, nil
				}
			}
			items = append(items, it)
		}
	}
	return importItems(p, kind, items, dry)
}

// checklistPrefix is the ID prefix for a feature's tasks: its name, up to 12 characters. When
// another source file already uses that prefix (two features starting with the same 12
// characters), a short hash of the file keeps the IDs apart.
func checklistPrefix(p *project.Project, feature, file string) string {
	prefix := strings.ToUpper(feature[:min(len(feature), 12)])
	tasks, _ := task.List(p)
	for _, t := range tasks {
		other, _ := t.Legacy["file"].(string)
		if other != "" && other != file && strings.HasPrefix(strings.ToUpper(t.ID), prefix+"-") {
			sum := sha256.Sum256([]byte(file))
			return strings.ToUpper(feature[:min(len(feature), 7)]) + "-" + strings.ToUpper(hex.EncodeToString(sum[:])[:4])
		}
	}
	return prefix
}

// syncSource checks off the checklist item an imported task came from (Spec Kit, OpenSpec,
// markdown). It reports whether the file changed.
func syncSource(p *project.Project, t *task.Task) (bool, error) {
	src, _ := t.Legacy["source"].(string)
	file, _ := t.Legacy["file"].(string)
	if (src != "spec-kit" && src != "openspec" && src != "markdown") || file == "" {
		return false, nil
	}
	clean := filepath.Clean(filepath.FromSlash(file))
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") || !strings.EqualFold(filepath.Ext(clean), ".md") {
		return false, nil // only markdown files inside the repository
	}
	path := p.Path(clean)
	// Never write through a symlink that leaves the repository.
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, nil
	}
	root, _ := filepath.EvalSymlinks(p.Root)
	if rel, err := filepath.Rel(root, real); err != nil || strings.HasPrefix(rel, "..") {
		return false, nil
	}
	b, err := os.ReadFile(real)
	if err != nil {
		return false, nil
	}
	ref, _ := t.Legacy["ref"].(string)
	item, _ := t.Legacy["item"].(string)
	if ref == "" && src == "spec-kit" { // imported by an older aitk: the ref is in the ID
		if m := refInID.FindStringSubmatch(t.ID); m != nil {
			ref = m[1]
		}
	}
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		m := checkbox.FindStringSubmatch(strings.TrimRight(l, "\r"))
		if m == nil {
			continue
		}
		text := strings.TrimSpace(m[2])
		var match bool
		switch {
		case ref != "":
			tm := taskIDToken.FindStringSubmatch(text)
			match = tm != nil && tm[1] == ref
		case item != "":
			match = text == item
		default:
			match = titleMatches(t.Title, text)
		}
		if !match || m[1] != " " {
			continue // keep looking: the same text may appear again, unchecked
		}
		lines[i] = strings.Replace(l, "[ ]", "[x]", 1)
		return true, fsx.WriteFileKeep(real, []byte(strings.Join(lines, "\n")), 0o644)
	}
	return false, nil
}

var refInID = regexp.MustCompile(`-(T\d{3,})$`)

// titleMatches compares a task title with a checklist item, allowing for Spec Kit markers in
// the item and for titles truncated to 120 characters.
func titleMatches(title, item string) bool {
	if tm := taskIDToken.FindStringSubmatch(item); tm != nil {
		item = strings.TrimSpace(tm[3])
	}
	if title == item {
		return true
	}
	if short, ok := strings.CutSuffix(title, "…"); ok {
		return strings.HasPrefix(item, short)
	}
	return false
}
