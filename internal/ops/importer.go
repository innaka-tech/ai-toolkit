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
}

var checkbox = regexp.MustCompile(`^\s*[-*] \[( |x|X)\] (.+)$`)

type importItem struct {
	id, title, objective string
	done                 bool
	status, profile      string // optional: explicit status; pinned profile
	criteria, tags       []string
	legacy               map[string]any
}

func importItems(p *project.Project, source string, items []importItem, dry bool) (*ImportResult, error) {
	res := &ImportResult{Source: source, Imported: []string{}, Skipped: []string{}, DryRun: dry}
	err := WithLock(p, func() error {
		for _, it := range items {
			if _, err := task.Find(p, it.id); err == nil {
				res.Skipped = append(res.Skipped, it.id)
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
				Created: now, Updated: now, CreatedBy: Tool(), Legacy: it.legacy}, Body: task.NewBody(it.objective, it.criteria)}
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

var taskIDToken = regexp.MustCompile(`^(T\d{3,})\b\s*(?:\[[^\]]*\]\s*)*(.*)$`)

// ImportChecklist imports Spec Kit (specs/*/tasks.md) or OpenSpec (openspec/changes/*/tasks.md)
// checklists: each checkbox line becomes a task; "T001 …" keeps its ID (prefixed by the spec name).
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
		n := 0
		for _, l := range strings.Split(string(b), "\n") {
			m := checkbox.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			n++
			text := strings.TrimSpace(m[2])
			id := fmt.Sprintf("%s-%d", strings.ToUpper(feature[:min(len(feature), 12)]), n)
			if tm := taskIDToken.FindStringSubmatch(text); tm != nil {
				id = strings.ToUpper(feature[:min(len(feature), 12)]) + "-" + tm[1]
				text = strings.TrimSpace(tm[2])
			}
			id = strings.Trim(regexp.MustCompile(`[^A-Za-z0-9-]+`).ReplaceAllString(id, "-"), "-")
			if !task.IDPattern.MatchString(id) {
				id = "S-" + id
			}
			items = append(items, importItem{id: id, title: text, done: m[1] != " ", objective: text,
				legacy: map[string]any{"source": kind, "file": filepath.ToSlash(rel)}})
		}
	}
	return importItems(p, kind, items, dry)
}
