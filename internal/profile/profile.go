// Package profile computes the risk profile and a working-tree fingerprint (docs/spec/workflow.md §5).
package profile

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

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

// Change is a changed file with its line count. From is the old path of a rename.
type Change struct {
	Path  string `json:"path"`
	Lines int    `json:"lines"`
	From  string `json:"from,omitempty"`
}

// Diff lists files changed between the merge base with the default branch and the
// working tree (including untracked files), excluding aitk metadata. Paths come from
// NUL-separated git output, so non-ASCII names are exact.
func Diff(p *project.Project) []Change { return DiffSince(p, "") }

// DiffSince is Diff that also includes everything changed since base (the commit a task
// started from). On the default branch the merge base is HEAD, so without base, work the task
// already committed would drop out of its risk profile.
func DiffSince(p *project.Project, base string) []Change {
	byPath := map[string]*Change{}
	mb := mergeBase(p)
	if mb == "" {
		// No commits yet: everything staged is new.
		out, _ := gitx.RunRaw(p.Root, "ls-files", "-z", "--cached")
		for _, rel := range strings.Split(string(out), "\x00") {
			if rel != "" {
				byPath[rel] = &Change{Path: rel, Lines: countLines(p.Path(rel))}
			}
		}
	} else {
		diffInto(p, mb, byPath)
		if base != "" && base != mb {
			if _, err := gitx.Run(p.Root, "merge-base", "--is-ancestor", base, "HEAD"); err == nil {
				diffInto(p, base, byPath)
			}
		}
	}
	untracked, _ := gitx.RunRaw(p.Root, "ls-files", "-z", "--others", "--exclude-standard")
	for _, rel := range strings.Split(string(untracked), "\x00") {
		if rel != "" {
			byPath[rel] = &Change{Path: rel, Lines: countLines(p.Path(rel))}
		}
	}
	var out []Change
	for path, c := range byPath {
		if !Managed(path) || c.From != "" && !Managed(c.From) {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// diffInto adds the changes from base to the working tree, keeping the larger line count when a
// path appears in more than one diff.
func diffInto(p *project.Project, base string, byPath map[string]*Change) {
	out, _ := gitx.RunRaw(p.Root, "diff", "--numstat", "-z", "-M", base)
	toks := strings.Split(string(out), "\x00")
	for i := 0; i < len(toks); i++ {
		f := strings.SplitN(toks[i], "\t", 3)
		if len(f) != 3 {
			continue
		}
		a, _ := strconv.Atoi(f[0]) // "-" for binary -> 0
		d, _ := strconv.Atoi(f[1])
		path, from := f[2], ""
		if path == "" && i+2 < len(toks) { // rename: "A\tD\t" NUL old NUL new
			from, path = toks[i+1], toks[i+2]
			i += 2
		}
		c := byPath[path]
		if c == nil {
			c = &Change{Path: path}
			byPath[path] = c
		}
		if a+d > c.Lines {
			c.Lines = a + d
		}
		if from != "" {
			c.From = from
		}
	}
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
			// Moving a file out of a sensitive path is a change to that path.
			if ok, _ := doublestar.PathMatch(pat, c.From); c.From != "" && ok {
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

// Fingerprint identifies the content of the code, aitk metadata excluded: the git tree the
// working tree would commit to. It changes with any edit and stays the same when the same
// content is committed, so committing after a passing check does not make the check stale.
// Symlink targets and submodule commits are part of it. When git cannot write objects (a
// sandbox with a read-only .git), it falls back to HEAD plus the changed files' content.
func Fingerprint(p *project.Project) string {
	if fp := treeFingerprint(p); fp != "" {
		return fp
	}
	return statusFingerprint(p)
}

func treeFingerprint(p *project.Project) string {
	gitDir, err := gitx.Run(p.Root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return ""
	}
	idx := filepath.Join(gitDir, "index")
	tmp, err := os.CreateTemp("", "aitk-index-*")
	if err != nil {
		return ""
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if b, err := os.ReadFile(idx); err == nil { // the real index keeps git's stat cache: only changed files are hashed
		if os.WriteFile(tmp.Name(), b, 0o600) != nil {
			return ""
		}
	} else {
		os.Remove(tmp.Name()) // no index yet: git creates the temporary one
	}
	env := append(os.Environ(), "GIT_INDEX_FILE="+tmp.Name(), "GIT_LITERAL_PATHSPECS=1")
	run := func(args ...string) (string, error) {
		c := exec.Command("git", args...)
		c.Dir, c.Env = p.Root, env
		out, err := c.Output()
		return strings.TrimSpace(string(out)), err
	}
	if _, err := run("add", "-A", "--", "."); err != nil {
		return ""
	}
	args := []string{"rm", "-r", "-q", "--cached", "--ignore-unmatch", "--"}
	for _, f := range []string{project.StateFile, project.ConfigFile, project.AgentsFile, "CLAUDE.md", ".gitattributes", project.DocsAI, project.ADRDir, project.LocalStateDir} {
		args = append(args, f)
	}
	if _, err := run(args...); err != nil {
		return ""
	}
	tree, err := run("write-tree")
	if err != nil || tree == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("tree:" + tree))
	return hex.EncodeToString(sum[:])
}

func statusFingerprint(p *project.Project) string {
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
		path := p.Path(rel)
		if target, err := os.Readlink(path); err == nil {
			h.Write([]byte("link:" + target))
		} else if info, err := os.Stat(path); err == nil && info.IsDir() {
			sub, _ := gitx.Run(path, "rev-parse", "HEAD") // a submodule
			h.Write([]byte("dir:" + sub))
		} else if b, err := os.ReadFile(path); err == nil {
			h.Write(b)
		} else {
			h.Write([]byte("<missing>"))
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Commits returns the short hashes of the task's commits: made after it was created (RFC 3339)
// and naming it, or no task, in their AI-Task trailer. Commit times are read per commit, so a
// commit with a skewed date does not hide the others.
func Commits(p *project.Project, since, id string) []string {
	var cs []string
	for _, c := range CommitsFull(p, since, id) {
		cs = append(cs, c[:min(len(c), 7)])
	}
	return cs
}

// CommitsFull is Commits with full hashes.
func CommitsFull(p *project.Project, since, id string) []string {
	created, err := time.Parse("2006-01-02T15:04:05Z", since)
	if err != nil || gitx.Head(p.Root) == "" {
		return nil
	}
	out, _ := gitx.RunRaw(p.Root, "log", "-n", "500", "--format=%H%x1f%ct%x1f%(trailers:key=AI-Task,valueonly,separator=%x2c)%x1e")
	var cs []string
	for _, rec := range strings.Split(string(out), "\x1e") {
		f := strings.Split(strings.TrimSpace(rec), "\x1f")
		if len(f) != 3 {
			continue
		}
		ct, _ := strconv.ParseInt(f[1], 10, 64)
		if ct <= created.Unix() { // the second the task was created belongs to the past
			continue
		}
		if ids := strings.TrimSpace(f[2]); ids != "" && id != "" && !hasFold(strings.Split(ids, ","), id) {
			continue // another task's commit
		}
		cs = append(cs, f[0])
	}
	return cs
}

func hasFold(xs []string, s string) bool {
	for _, x := range xs {
		if strings.EqualFold(strings.TrimSpace(x), s) {
			return true
		}
	}
	return false
}
