// Package profile computes the risk profile and a working-tree fingerprint (docs/spec/workflow.md §5).
package profile

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path"
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
	return adapterFile(rel)
}

// adapterFiles and adapterPrefixes are what aitk adapters sync writes for AI tools: aitk's own
// rules, prompts and skills, and the tool settings it registers the MCP server and session hook
// in. Tool configuration, not project code.
var adapterFiles = map[string]bool{
	".mcp.json": true, ".claude/settings.json": true, ".gemini/settings.json": true, ".qwen/settings.json": true,
	".cursor/mcp.json": true, ".kiro/settings/mcp.json": true, ".roo/mcp.json": true, ".vscode/mcp.json": true,
	"opencode.json": true, ".aider.conf.yml": true, ".github/copilot-instructions.md": true, ".junie/guidelines.md": true,
	".clinerules": true,
}

var adapterPrefixes = []string{".claude/skills/aitk/", ".clinerules/aitk", ".cursor/rules/aitk", ".github/prompts/aitk-",
	".kiro/steering/aitk", ".roo/rules/aitk", ".windsurf/rules/aitk", ".windsurf/workflows/aitk-", ".opencode/command/aitk-"}

func adapterFile(rel string) bool {
	if adapterFiles[rel] {
		return true
	}
	for _, p := range adapterPrefixes {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}

// ManagedIn is Managed for a path as git reports it: aitk's own directories may sit behind a
// symlink inside the repository (docs -> documentation), where git names the target's path.
func ManagedIn(p *project.Project, rel string) bool {
	return Managed(rel) || Managed(p.AitkRel(rel))
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
		if base != "" && base != mb && base == EmptyTree(p) {
			diffInto(p, base, byPath) // everything since the first commit
		} else if base != "" && base != mb {
			// After an amend or a rebase the base is no longer an ancestor of HEAD: diff from the
			// point where the two histories meet, which still covers the task's work.
			if _, err := gitx.Run(p.Root, "merge-base", "--is-ancestor", base, "HEAD"); err != nil {
				base, _ = gitx.Run(p.Root, "merge-base", base, "HEAD")
			}
			if base != "" && base != mb {
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
		if !ManagedIn(p, path) || c.From != "" && !ManagedIn(p, c.From) {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// EmptyTree is git's empty tree object (for the object format of this repository): the base
// to diff from when the work starts with the very first commit.
func EmptyTree(p *project.Project) string {
	c := exec.Command("git", "hash-object", "-t", "tree", "--stdin")
	c.Dir = p.Root
	c.Stdin = strings.NewReader("")
	out, err := c.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
	// Of the local and remote-tracking default branch, use the merge base nearest HEAD: a
	// stale local branch would otherwise count work already merged upstream as this change.
	best := ""
	for _, ref := range []string{p.Config.DefaultBranch(), "origin/" + p.Config.DefaultBranch()} {
		mb, err := gitx.Run(p.Root, "merge-base", ref, "HEAD")
		if err != nil || mb == "" {
			continue
		}
		if best == "" {
			best = mb
		} else if _, err := gitx.Run(p.Root, "merge-base", "--is-ancestor", best, mb); err == nil {
			best = mb
		}
	}
	if best == "" {
		return head
	}
	return best
}

func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	head := make([]byte, 8000)
	k, _ := f.Read(head)
	if bytes.IndexByte(head[:k], 0) >= 0 {
		return 0 // binary: lines mean nothing (git's numstat reports "-" too)
	}
	f.Seek(0, 0)
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

// Fresh reports whether a fingerprint recorded earlier still describes the code. Both kinds are
// accepted, so a check recorded where .git could not be written (or the other way round) still
// matches.
func Fresh(p *project.Project, recorded string) bool {
	if recorded == "" {
		return false
	}
	if fp := treeFingerprint(p); fp != "" && fp == recorded {
		return true
	}
	return statusFingerprint(p) == recorded
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
	// New objects go to a throwaway directory (the repository's own objects stay readable as an
	// alternate), so a check writes nothing into .git and works when .git is read-only.
	objDir, err := os.MkdirTemp("", "aitk-objects-*")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(objDir)
	realObjects, err := gitx.Run(p.Root, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return ""
	}
	alternates := realObjects
	if prev := os.Getenv("GIT_ALTERNATE_OBJECT_DIRECTORIES"); prev != "" {
		alternates += string(os.PathListSeparator) + prev
	}
	env := append(os.Environ(), "GIT_INDEX_FILE="+tmp.Name(), "GIT_LITERAL_PATHSPECS=1",
		"GIT_OBJECT_DIRECTORY="+objDir, "GIT_ALTERNATE_OBJECT_DIRECTORIES="+alternates)
	run := func(args ...string) (string, error) {
		c := exec.Command("git", args...)
		c.Dir, c.Env = p.Root, env
		out, err := c.Output()
		return strings.TrimSpace(string(out)), err
	}
	// Very large untracked files are fingerprinted by size and modification time instead of
	// being copied into the temporary object store on every check.
	big := bigUntracked(p, 32<<20)
	if len(big) > 0 {
		ex, err := os.CreateTemp("", "aitk-exclude-*")
		if err != nil {
			return ""
		}
		for _, f := range big {
			fmt.Fprintf(ex, "/%s\n", f.path)
		}
		ex.Close()
		defer os.Remove(ex.Name())
		run = func(args ...string) (string, error) {
			c := exec.Command("git", append([]string{"-c", "core.excludesFile=" + ex.Name()}, args...)...)
			c.Dir, c.Env = p.Root, env
			out, err := c.Output()
			return strings.TrimSpace(string(out)), err
		}
	}
	if _, err := run("add", "-A", "--", "."); err != nil {
		return ""
	}
	args := []string{"rm", "-r", "-q", "--cached", "--ignore-unmatch", "--"}
	// Files at the root are named as they are (git tracks a symlinked CLAUDE.md as the link
	// itself); aitk's directories by the path git tracks them under, also behind an in-repo
	// symlink, plus their own name when no parent is a link (a link at docs/ai is a blob there).
	args = append(args, project.StateFile, project.ConfigFile, project.AgentsFile, "CLAUDE.md", ".gitattributes")
	for _, d := range []string{project.DocsAI, project.ADRDir, project.LocalStateDir} {
		g := p.GitRel(d)
		args = append(args, g)
		if parent := path.Dir(d); g != d && (parent == "." || p.GitRel(parent) == parent) {
			args = append(args, d)
		}
	}
	if _, err := run(args...); err != nil {
		return ""
	}
	// The AI tools' files aitk adapters writes are not code either.
	if listed, err := run("ls-files", "-z"); err == nil {
		var tools []string
		for _, rel := range strings.Split(listed, "\x00") {
			if adapterFile(rel) {
				tools = append(tools, rel)
			}
		}
		if len(tools) > 0 {
			if _, err := run(append([]string{"rm", "-q", "--cached", "--ignore-unmatch", "--"}, tools...)...); err != nil {
				return ""
			}
		}
	}
	tree, err := run("write-tree")
	if err != nil || tree == "" {
		return ""
	}
	h := sha256.New()
	h.Write([]byte("tree:" + tree))
	for _, f := range big {
		fmt.Fprintf(h, "\x00big:%s:%d:%d", f.path, f.size, f.mtime)
	}
	if _, err := os.Stat(p.Path(".gitmodules")); err == nil {
		// Uncommitted work inside submodules is not in the tree (only their commits are).
		dirt, _ := gitx.RunRaw(p.Root, "submodule", "foreach", "--recursive", "--quiet", "git status --porcelain=v1 --untracked-files=all; git diff HEAD")
		h.Write(dirt)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type bigFile struct {
	path        string
	size, mtime int64
}

// bigUntracked lists untracked, not ignored files larger than limit bytes.
func bigUntracked(p *project.Project, limit int64) []bigFile {
	out, _ := gitx.RunRaw(p.Root, "ls-files", "-z", "--others", "--exclude-standard")
	var big []bigFile
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" || ManagedIn(p, rel) {
			continue
		}
		if info, err := os.Lstat(p.Path(rel)); err == nil && info.Mode().IsRegular() && info.Size() > limit {
			big = append(big, bigFile{rel, info.Size(), info.ModTime().UnixNano()})
		}
	}
	return big
}

func statusFingerprint(p *project.Project) string {
	h := sha256.New()
	h.Write([]byte(gitx.Head(p.Root)))
	h.Write([]byte{0})
	out, _ := gitx.RunRaw(p.Root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames")
	var paths []string
	for _, rec := range strings.Split(string(out), "\x00") {
		if len(rec) > 3 {
			if rel := rec[3:]; !ManagedIn(p, rel) {
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
