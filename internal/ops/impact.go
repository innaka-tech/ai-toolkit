package ops

import (
	"os"
	"path"
	"sort"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/profile"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
)

// ImpactFile is one changed source file and the code that depends on it.
type ImpactFile struct {
	Path         string   `json:"path"`
	Lines        int      `json:"lines"`
	ReferencedBy []string `json:"referenced_by"`
	Tests        []string `json:"tests"` // tests that reference it, sit next to it, or were changed with it
	More         int      `json:"more,omitempty"`
}

// ImpactResult lists the blast radius of the current change.
type ImpactResult struct {
	Files        []ImpactFile `json:"files"`
	TestsChanged []string     `json:"tests_changed"`
	Untested     []string     `json:"untested"` // changed source files no test appears to cover
}

var codeExt = map[string]bool{".go": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".cjs": true, ".vue": true, ".svelte": true,
	".py": true, ".rb": true, ".php": true, ".java": true, ".kt": true, ".cs": true, ".rs": true, ".swift": true, ".scala": true, ".c": true, ".h": true,
	".cc": true, ".cpp": true, ".hpp": true, ".dart": true, ".ex": true, ".exs": true, ".sql": true, ".sh": true}

// genericStems are file names too common to find references by name.
var genericStems = map[string]bool{"index": true, "main": true, "init": true, "__init__": true, "mod": true, "lib": true, "app": true, "types": true, "util": true, "utils": true, "helpers": true, "common": true, "constants": true, "config": true}

const maxRefs = 20

// Impact finds, for each changed source file, which tracked files reference it (by module name,
// or by Go package import path) and which tests cover it. It is a heuristic over text search:
// a fast way to see what a change can break, not a call graph.
func Impact(p *project.Project) *ImpactResult {
	res := &ImpactResult{Files: []ImpactFile{}, TestsChanged: []string{}, Untested: []string{}}
	changes := profile.Diff(p)
	changedTests := map[string]bool{}
	type src struct {
		path, ext, dir, stem, imp string
		lines                     int
	}
	var sources []src
	goModule := goModulePath(p)
	for _, c := range changes {
		if IsTestPath(c.Path) {
			res.TestsChanged = append(res.TestsChanged, c.Path)
			changedTests[c.Path] = true
			continue
		}
		ext := strings.ToLower(path.Ext(c.Path))
		if !codeExt[ext] || strings.HasPrefix(c.Path, "docs/") {
			continue
		}
		if _, err := os.Stat(p.Path(c.Path)); err != nil {
			continue // deleted: nothing left to cover
		}
		s := src{path: c.Path, ext: ext, dir: path.Dir(c.Path), stem: strings.TrimSuffix(path.Base(c.Path), path.Ext(c.Path)), lines: c.Lines}
		if ext == ".go" && goModule != "" {
			s.imp = goModule
			if s.dir != "." {
				s.imp += "/" + s.dir
			}
		}
		sources = append(sources, s)
		if len(sources) == maxImpactFiles {
			break
		}
	}
	// Two searches for the whole change: file names as words, and Go import paths.
	var stems, imports []string
	for _, s := range sources {
		if len(s.stem) >= 4 && !genericStems[strings.ToLower(s.stem)] {
			stems = append(stems, s.stem)
		}
		if s.imp != "" {
			imports = append(imports, `"`+s.imp+`"`)
		}
	}
	byStem := grepMatches(p, true, stems)
	byImport := grepMatches(p, false, imports)
	tracked := trackedFiles(p)
	for _, s := range sources {
		f := ImpactFile{Path: s.path, Lines: s.lines, ReferencedBy: []string{}, Tests: []string{}}
		refs := map[string]bool{}
		if len(s.stem) >= 4 && !genericStems[strings.ToLower(s.stem)] {
			for _, r := range byStem[s.stem] {
				refs[r] = true
			}
		}
		if s.imp != "" {
			for _, r := range byImport[`"`+s.imp+`"`] {
				refs[r] = true
			}
		}
		delete(refs, s.path)
		for r := range refs {
			if !codeExt[strings.ToLower(path.Ext(r))] {
				delete(refs, r) // docs that mention a file do not depend on it
			}
		}
		tests := map[string]bool{}
		for r := range refs {
			if IsTestPath(r) {
				tests[r] = true
			}
		}
		for _, t := range tracked {
			if IsTestPath(t) && path.Dir(t) == s.dir && path.Ext(t) == s.ext && sameUnit(t, s.path, s.ext) {
				tests[t] = true
			}
		}
		for t := range changedTests {
			if path.Dir(t) == s.dir || len(s.stem) >= 4 && strings.Contains(path.Base(t), s.stem) {
				tests[t] = true
			}
		}
		for r := range refs {
			f.ReferencedBy = append(f.ReferencedBy, r)
		}
		sort.Strings(f.ReferencedBy)
		if len(f.ReferencedBy) > maxRefs {
			f.More = len(f.ReferencedBy) - maxRefs
			f.ReferencedBy = f.ReferencedBy[:maxRefs]
		}
		for t := range tests {
			f.Tests = append(f.Tests, t)
		}
		sort.Strings(f.Tests)
		if len(f.Tests) == 0 {
			res.Untested = append(res.Untested, s.path)
		}
		res.Files = append(res.Files, f)
	}
	return res
}

// maxImpactFiles bounds the work on very large changes.
const maxImpactFiles = 300

// sameUnit: in Go every _test.go file of the package covers it; elsewhere the test is named after the file.
func sameUnit(test, file, ext string) bool {
	if ext == ".go" {
		return true
	}
	stem := strings.TrimSuffix(path.Base(file), ext)
	return strings.Contains(strings.ToLower(path.Base(test)), strings.ToLower(stem))
}

// grepMatches searches tracked and untracked files once for all patterns and returns, per
// pattern, the files containing it. Paths are relative to the repository root.
func grepMatches(p *project.Project, words bool, patterns []string) map[string][]string {
	found := map[string][]string{}
	if len(patterns) == 0 {
		return found
	}
	args := []string{"grep", "--untracked", "--full-name", "-I", "-F", "-o", "-z"}
	if words {
		args = append(args, "-w")
	}
	for _, pt := range patterns {
		args = append(args, "-e", pt)
	}
	args = append(args, "--", ".", ":(exclude)docs/ai", ":(exclude)vendor", ":(exclude)node_modules")
	out, _ := gitx.RunRaw(p.Root, args...)
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		file, match, ok := strings.Cut(line, "\x00")
		if !ok || seen[file+"\x00"+match] {
			continue
		}
		seen[file+"\x00"+match] = true
		found[match] = append(found[match], file)
	}
	return found
}

func trackedFiles(p *project.Project) []string {
	out, _ := gitx.RunRaw(p.Root, "ls-files", "-z", "--full-name", "--cached", "--others", "--exclude-standard")
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

func goModulePath(p *project.Project) string {
	b, err := os.ReadFile(p.Path("go.mod"))
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "module" {
			return f[1]
		}
	}
	return ""
}
