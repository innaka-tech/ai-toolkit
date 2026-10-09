// Package profile computes the risk profile and a working-tree fingerprint (docs/spec/workflow.md §5).
package profile

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/innaka-tech/ai-toolkit/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/internal/project"
)

// Managed reports whether rel is maintained by aitk itself (excluded from diffs and fingerprints).
func Managed(rel string) bool {
	return rel == project.StateFile || strings.HasPrefix(rel, project.DocsAI+"/")
}

// Change is a changed file with its line count.
type Change struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
}

// Diff lists files changed between the merge base with the default branch and the
// working tree (including untracked files), excluding aitk-managed files.
func Diff(p *project.Project) []Change {
	base := mergeBase(p)
	byPath := map[string]int{}
	if base != "" {
		out, _ := gitx.Run(p.Root, "diff", "--numstat", base)
		for _, l := range strings.Split(out, "\n") {
			f := strings.SplitN(l, "\t", 3)
			if len(f) != 3 {
				continue
			}
			a, _ := strconv.Atoi(f[0]) // "-" for binary -> 0
			d, _ := strconv.Atoi(f[1])
			byPath[renamed(f[2])] += a + d
		}
	}
	untracked, _ := gitx.Run(p.Root, "ls-files", "--others", "--exclude-standard")
	for _, rel := range strings.Split(untracked, "\n") {
		if rel == "" {
			continue
		}
		byPath[rel] += countLines(p.Path(rel))
	}
	var out []Change
	for path, n := range byPath {
		if !Managed(path) {
			out = append(out, Change{path, n})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func renamed(s string) string {
	// numstat shows renames as "old => new" or "dir/{old => new}/f"
	if i := strings.Index(s, "{"); i >= 0 {
		j := strings.Index(s, "}")
		if j > i {
			inner := s[i+1 : j]
			if k := strings.Index(inner, " => "); k >= 0 {
				return s[:i] + inner[k+4:] + s[j+1:]
			}
		}
	}
	if k := strings.Index(s, " => "); k >= 0 {
		return s[k+4:]
	}
	return s
}

func mergeBase(p *project.Project) string {
	head := gitx.Head(p.Root)
	if head == "" {
		return ""
	}
	for _, ref := range []string{p.Config.DefaultBranch(), "origin/" + p.Config.DefaultBranch()} {
		if mb, err := gitx.Run(p.Root, "merge-base", ref, "HEAD"); err == nil && mb != "" {
			return mb
		}
	}
	return head
}

func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for s.Scan() {
		n++
	}
	return n
}

// Compute returns the profile for the current changes ("lite", "standard", "strict").
func Compute(p *project.Project, changes []Change) string {
	lines := 0
	for _, c := range changes {
		for _, pat := range p.Config.Risk.SensitivePaths {
			if ok, _ := doublestar.PathMatch(pat, c.Path); ok {
				return "strict"
			}
		}
		lines += c.Lines
	}
	if len(changes) <= p.Config.LiteMaxFiles() && lines <= p.Config.LiteMaxLines() {
		return "lite"
	}
	return "standard"
}

// Fingerprint hashes HEAD, the tracked diff, and untracked file contents, excluding
// aitk-managed files. Equal fingerprints mean the code has not changed.
func Fingerprint(p *project.Project) string {
	h := sha256.New()
	h.Write([]byte(gitx.Head(p.Root)))
	diff, _ := gitx.RunRaw(p.Root, "diff", "HEAD", "--binary", "--", ".", ":(exclude)"+project.DocsAI, ":(exclude)"+project.StateFile)
	if gitx.Head(p.Root) == "" {
		diff, _ = gitx.RunRaw(p.Root, "diff", "--cached", "--binary")
	}
	h.Write(diff)
	untracked, _ := gitx.Run(p.Root, "ls-files", "--others", "--exclude-standard")
	files := strings.Split(untracked, "\n")
	sort.Strings(files)
	for _, rel := range files {
		if rel == "" || Managed(rel) {
			continue
		}
		b, _ := os.ReadFile(p.Path(rel))
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Commits returns commit hashes made since the given RFC 3339 time on the current branch.
func Commits(p *project.Project, since string) []string {
	if since == "" || gitx.Head(p.Root) == "" {
		return nil
	}
	out, _ := gitx.Run(p.Root, "log", "--format=%h", "--since="+since)
	var cs []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			cs = append(cs, l)
		}
	}
	return cs
}
