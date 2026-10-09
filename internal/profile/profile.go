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

	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
)

// Managed reports whether rel is aitk's own metadata rather than project code. Such files
// do not count toward the risk profile and do not make a passing check stale.
func Managed(rel string) bool {
	switch rel {
	case project.StateFile, project.ConfigFile, project.AgentsFile, "CLAUDE.md", ".gitattributes":
		return true
	}
	for _, dir := range []string{project.DocsAI + "/", project.ADRDir + "/", project.LocalStateDir + "/"} {
		if strings.HasPrefix(rel, dir) {
			return true
		}
	}
	return false
}

// Change is a changed file with its line count.
type Change struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
}

// Diff lists files changed between the merge base with the default branch and the
// working tree (including untracked files), excluding aitk metadata. Paths come from
// NUL-separated git output, so non-ASCII names are exact.
func Diff(p *project.Project) []Change {
	byPath := map[string]int{}
	if base := mergeBase(p); base != "" {
		out, _ := gitx.RunRaw(p.Root, "diff", "--numstat", "-z", base)
		toks := strings.Split(string(out), "\x00")
		for i := 0; i < len(toks); i++ {
			f := strings.SplitN(toks[i], "\t", 3)
			if len(f) != 3 {
				continue
			}
			a, _ := strconv.Atoi(f[0]) // "-" for binary -> 0
			d, _ := strconv.Atoi(f[1])
			path := f[2]
			if path == "" && i+2 < len(toks) { // rename: "A\tD\t" NUL old NUL new
				path = toks[i+2]
				i += 2
			}
			byPath[path] += a + d
		}
	} else {
		// No commits yet: everything staged is new.
		out, _ := gitx.RunRaw(p.Root, "ls-files", "-z", "--cached")
		for _, rel := range strings.Split(string(out), "\x00") {
			if rel != "" {
				byPath[rel] += countLines(p.Path(rel))
			}
		}
	}
	untracked, _ := gitx.RunRaw(p.Root, "ls-files", "-z", "--others", "--exclude-standard")
	for _, rel := range strings.Split(string(untracked), "\x00") {
		if rel != "" {
			byPath[rel] += countLines(p.Path(rel))
		}
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

// Fingerprint identifies the state of the code: HEAD plus the path and current content of
// every changed, staged, or untracked file (aitk metadata excluded). Any edit to code after
// a check changes it, with or without commits, whatever the file names are.
func Fingerprint(p *project.Project) string {
	h := sha256.New()
	h.Write([]byte(gitx.Head(p.Root)))
	h.Write([]byte{0})
	out, _ := gitx.RunRaw(p.Root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames")
	var paths []string
	for _, rec := range strings.Split(string(out), "\x00") {
		if len(rec) > 3 {
			if rel := rec[3:]; !Managed(rel) {
				paths = append(paths, rel)
			}
		}
	}
	sort.Strings(paths)
	for _, rel := range paths {
		h.Write([]byte(rel))
		h.Write([]byte{0})
		if b, err := os.ReadFile(p.Path(rel)); err == nil {
			h.Write(b)
		} else {
			h.Write([]byte("<missing>"))
		}
		h.Write([]byte{0})
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
