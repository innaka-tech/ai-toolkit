package ops

import (
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
	dependsOn            []string
	legacy               map[string]any
}

func importItems(p *project.Project, source string, items []importItem, dry bool) (*ImportResult, error) {
	res := &ImportResult{Source: source, Imported: []string{}, Skipped: []string{}, DryRun: dry}
	err := WithLock(p, func() error {
		for _, it := range items {
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
		for _, l := range strings.Split(is.Body, "\n") {
			if m := checkbox.FindStringSubmatch(l); m != nil {
				it.criteria = append(it.criteria, strings.TrimSpace(m[2]))
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
		barrier, group := "", []string(nil)
		for _, l := range strings.Split(string(b), "\n") {
			m := checkbox.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			n++
			item := strings.TrimSpace(m[2])
			text := item
			prefix := strings.ToUpper(feature[:min(len(feature), 12)])
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
				if parallel {
					if barrier != "" {
						it.dependsOn = []string{barrier}
					}
					group = append(group, id)
				} else {
					if len(group) > 0 {
						it.dependsOn = group
					} else if barrier != "" {
						it.dependsOn = []string{barrier}
					}
					barrier, group = id, nil
				}
			}
			items = append(items, it)
		}
	}
	return importItems(p, kind, items, dry)
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
	b, err := os.ReadFile(path)
	if err != nil {
		return false, nil
	}
	ref, _ := t.Legacy["ref"].(string)
	item, _ := t.Legacy["item"].(string)
	if item == "" {
		item = t.Title
	}
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		m := checkbox.FindStringSubmatch(strings.TrimRight(l, "\r"))
		if m == nil {
			continue
		}
		text := strings.TrimSpace(m[2])
		match := text == item
		if ref != "" {
			tm := taskIDToken.FindStringSubmatch(text)
			match = tm != nil && tm[1] == ref
		}
		if !match {
			continue
		}
		if m[1] != " " {
			return false, nil
		}
		lines[i] = strings.Replace(l, "[ ]", "[x]", 1)
		return true, fsx.WriteFileKeep(path, []byte(strings.Join(lines, "\n")), 0o644)
	}
	return false, nil
}
