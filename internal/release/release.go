// Package release versions the project aitk manages: the next SemVer from Conventional
// Commits, a Keep a Changelog section, manifest versions, and an annotated tag.
package release

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
)

// Version is a SemVer 2.0 version.
type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

var semverRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

// Parse reads "1.2.3" or "1.2.3-rc.1".
func Parse(s string) (Version, error) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("not a SemVer version: %q", s)
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	c, _ := strconv.Atoi(m[3])
	return Version{a, b, c, m[4]}, nil
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Commit is a parsed Conventional Commit.
type Commit struct {
	Hash     string `json:"hash"`
	Type     string `json:"type"`
	Scope    string `json:"scope,omitempty"`
	Subject  string `json:"subject"`
	Breaking bool   `json:"breaking,omitempty"`
	Task     string `json:"task,omitempty"`
}

var headerRe = regexp.MustCompile(`^([a-z]+)(?:\(([^)]+)\))?(!)?: (.+)$`)
var taskTrailer = regexp.MustCompile(`(?m)^AI-Task: *([A-Za-z][A-Za-z0-9-]{0,31})\s*$`)

var quotedRe = regexp.MustCompile(`^(Revert|Reapply) "(.+)"$`)

func parseCommit(hash, subject, body string) (Commit, bool) {
	subject = strings.TrimSpace(subject)
	// git's default messages: Revert "<header>" undoes a change (a fix for the release notes);
	// Reapply "<header>" brings it back (the original change again).
	if q := quotedRe.FindStringSubmatch(subject); q != nil {
		inner, ok := parseCommit(hash, q[2], "")
		if q[1] == "Reapply" {
			if ok {
				inner.Task = taskOf(body)
			}
			return inner, ok
		}
		c := Commit{Hash: hash, Type: "revert", Subject: q[2], Task: taskOf(body)}
		if ok {
			c.Scope = inner.Scope
		}
		return c, true
	}
	m := headerRe.FindStringSubmatch(subject)
	if m == nil {
		return Commit{}, false
	}
	c := Commit{Hash: hash, Type: m[1], Scope: m[2], Subject: m[4], Breaking: m[3] == "!"}
	if strings.Contains(body, "BREAKING CHANGE:") || strings.Contains(body, "BREAKING-CHANGE:") {
		c.Breaking = true
	}
	c.Task = taskOf(body)
	return c, true
}

func taskOf(body string) string {
	if t := taskTrailer.FindStringSubmatch(body); t != nil {
		return t[1]
	}
	return ""
}

// Plan describes a release.
type Plan struct {
	Previous     string   `json:"previous"`
	Version      string   `json:"version"`
	Tag          string   `json:"tag"`
	Bump         string   `json:"bump"`
	Commits      []Commit `json:"commits"`
	Skipped      int      `json:"skipped_non_conventional"`
	Notes        string   `json:"notes"`
	Changelog    string   `json:"changelog"`
	Manifests    []string `json:"manifests,omitempty"`
	Date         string   `json:"date"`
	FirstRelease bool     `json:"first_release,omitempty"`
}

// Options control the plan.
type Options struct {
	Bump string // auto | major | minor | patch
	Pre  string // pre-release label, e.g. rc
}

// LastTag returns the latest release tag reachable from HEAD and its version.
// Tags that are not SemVer (v2-beta, nightly) are ignored rather than ending the search.
func LastTag(p *project.Project) (string, Version, bool) {
	prefix := p.Config.TagPrefix()
	out, err := gitx.Run(p.Root, "tag", "--merged", "HEAD", "--list", prefix+"[0-9]*")
	if err != nil {
		return "", Version{}, false
	}
	best, bestTag, found := Version{}, "", false
	for _, tag := range strings.Fields(out) {
		v, err := Parse(strings.TrimPrefix(tag, prefix))
		if err != nil {
			continue
		}
		if !found || Less(best, v) {
			best, bestTag, found = v, tag, true
		}
	}
	return bestTag, best, found
}

// Less orders versions by SemVer precedence (a pre-release comes before its release).
func Less(a, b Version) bool {
	if a.Major != b.Major {
		return a.Major < b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor < b.Minor
	}
	if a.Patch != b.Patch {
		return a.Patch < b.Patch
	}
	switch {
	case a.Pre == b.Pre:
		return false
	case a.Pre == "":
		return false
	case b.Pre == "":
		return true
	}
	ap, bp := strings.Split(a.Pre, "."), strings.Split(b.Pre, ".")
	for i := 0; i < len(ap) && i < len(bp); i++ {
		if ap[i] == bp[i] {
			continue
		}
		ai, aerr := strconv.Atoi(ap[i])
		bi, berr := strconv.Atoi(bp[i])
		switch {
		case aerr == nil && berr == nil:
			return ai < bi
		case aerr == nil:
			return true // numeric identifiers sort before alphanumeric ones
		case berr == nil:
			return false
		}
		return ap[i] < bp[i]
	}
	return len(ap) < len(bp)
}

// Make computes the release plan.
func Make(p *project.Project, o Options) (*Plan, error) {
	prefix := p.Config.TagPrefix()
	tag, prev, found := LastTag(p)
	rng := "HEAD"
	if found {
		rng = tag + "..HEAD"
	}
	out, err := gitx.Run(p.Root, "log", "--no-merges", "--format=%H%x1f%s%x1f%b%x1e", rng)
	if err != nil {
		return nil, err
	}
	plan := &Plan{Date: time.Now().Format("2006-01-02"), Changelog: p.Config.ChangelogFile(), FirstRelease: !found}
	for _, rec := range strings.Split(out, "\x1e") {
		f := strings.SplitN(strings.TrimLeft(rec, "\n"), "\x1f", 3)
		if len(f) < 2 {
			continue
		}
		body := ""
		if len(f) == 3 {
			body = f[2]
		}
		c, ok := parseCommit(f[0][:min(7, len(f[0]))], f[1], body)
		if !ok {
			plan.Skipped++
			continue
		}
		if c.Type == "chore" && c.Scope == "release" {
			continue
		}
		plan.Commits = append(plan.Commits, c)
	}
	bump := o.Bump
	if bump == "" || bump == "auto" {
		bump = autoBump(plan.Commits, prev, found)
	}
	if bump == "" && found {
		return plan, fmt.Errorf("nothing to release: no feat, fix, perf, or breaking commits since %s", orStart(tag))
	}
	if !found && len(plan.Commits) == 0 && plan.Skipped == 0 {
		return plan, fmt.Errorf("nothing to release: no commits yet")
	}
	next := prev
	if !found {
		iv, err := Parse(p.Config.InitialVersion())
		if err != nil {
			return nil, err
		}
		next = iv
		bump = "initial"
	} else {
		next.Pre = ""
		if prev.Pre != "" {
			// From a pre-release of X.Y.Z, any change already covered by X.Y.Z stays X.Y.Z.
			base := Version{prev.Major, prev.Minor, prev.Patch, ""}
			switch {
			case bump == "major" && (prev.Minor > 0 || prev.Patch > 0):
				base = Version{prev.Major + 1, 0, 0, ""}
			case bump == "minor" && prev.Patch > 0:
				base = Version{prev.Major, prev.Minor + 1, 0, ""}
			}
			next, bump = base, bump+" (from pre-release)"
			goto done
		}
		switch bump {
		case "major":
			next = Version{prev.Major + 1, 0, 0, ""}
		case "minor":
			next = Version{prev.Major, prev.Minor + 1, 0, ""}
		case "patch":
			next = Version{prev.Major, prev.Minor, prev.Patch + 1, ""}
		default:
			return nil, fmt.Errorf("invalid bump %q (auto, major, minor, patch)", o.Bump)
		}
	done:
	}
	if o.Pre != "" {
		next.Pre = o.Pre + "." + strconv.Itoa(nextPre(p, prefix, next, o.Pre))
	}
	plan.Previous, plan.Version, plan.Tag, plan.Bump = strings.TrimPrefix(tag, prefix), next.String(), prefix+next.String(), bump
	if _, err := gitx.Run(p.Root, "rev-parse", "-q", "--verify", "refs/tags/"+plan.Tag); err == nil {
		return plan, fmt.Errorf("tag %s already exists", plan.Tag)
	}
	plan.Notes = notes(plan)
	plan.Manifests = manifests(p)
	return plan, nil
}

func orStart(tag string) string {
	if tag == "" {
		return "the first commit"
	}
	return tag
}

func autoBump(cs []Commit, prev Version, found bool) string {
	rank := 0
	for _, c := range cs {
		r := 0
		switch {
		case c.Breaking:
			r = 3
		case c.Type == "feat":
			r = 2
		case c.Type == "fix" || c.Type == "perf" || c.Type == "revert":
			r = 1
		}
		if r > rank {
			rank = r
		}
	}
	if rank == 3 && found && prev.Major == 0 {
		rank = 2 // SemVer 0.x: breaking changes bump the minor version
	}
	return map[int]string{1: "patch", 2: "minor", 3: "major"}[rank]
}

func nextPre(p *project.Project, prefix string, v Version, label string) int {
	out, _ := gitx.Run(p.Root, "tag", "--list", prefix+fmt.Sprintf("%d.%d.%d-%s.*", v.Major, v.Minor, v.Patch, label))
	n := 0
	for _, t := range strings.Fields(out) {
		parts := strings.Split(t, ".")
		if k, err := strconv.Atoi(parts[len(parts)-1]); err == nil && k > n {
			n = k
		}
	}
	return n + 1
}

// notes renders the Keep a Changelog section.
func notes(pl *Plan) string {
	groups := map[string][]string{}
	order := []string{"⚠ Breaking", "Added", "Changed", "Fixed", "Security"}
	for _, c := range pl.Commits {
		line := c.Subject
		if c.Scope != "" {
			line = "**" + c.Scope + ":** " + line
		}
		line += " (" + c.Hash
		if c.Task != "" {
			line += ", " + c.Task
		}
		line += ")"
		g := ""
		switch {
		case c.Breaking:
			g = "⚠ Breaking"
		case c.Scope == "security" || c.Type == "security" || strings.Contains(strings.ToLower(c.Subject), "vulnerab"):
			g = "Security"
		case c.Type == "feat":
			g = "Added"
		case c.Type == "fix":
			g = "Fixed"
		case c.Type == "perf" || c.Type == "refactor" || c.Type == "revert":
			g = "Changed"
		default:
			continue // docs, test, chore, ci, build, style: not user-facing
		}
		groups[g] = append(groups[g], "- "+line)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## [%s] - %s\n", pl.Version, pl.Date)
	for _, g := range order {
		if len(groups[g]) == 0 {
			continue
		}
		sort.Strings(groups[g])
		fmt.Fprintf(&b, "\n### %s\n\n%s\n", g, strings.Join(groups[g], "\n"))
	}
	if len(groups) == 0 {
		b.WriteString("\nMaintenance release.\n")
	}
	return b.String()
}

const changelogHeader = `# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

`

// WriteChangelog inserts the section after "## [Unreleased]" (creating the file when absent).
func WriteChangelog(p *project.Project, pl *Plan) error {
	path := p.Path(pl.Changelog)
	b, _ := os.ReadFile(path)
	s := string(b)
	if s == "" {
		s = changelogHeader
	}
	if strings.Contains(s, "## ["+pl.Version+"]") {
		return fmt.Errorf("%s already has a section for %s", pl.Changelog, pl.Version)
	}
	lines := strings.SplitAfter(s, "\n")
	insert := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "## [") && !strings.HasPrefix(strings.ToLower(l), "## [unreleased]") {
			insert = i
			break
		}
	}
	section := pl.Notes + "\n"
	if insert < 0 {
		s = strings.TrimRight(s, "\n") + "\n\n" + section
	} else {
		s = strings.Join(lines[:insert], "") + section + strings.Join(lines[insert:], "")
	}
	return fsx.WriteFileKeep(path, []byte(s), 0o644)
}

var tomlVersion = regexp.MustCompile(`^version\s*=\s*"[^"]*"`)

// tomlSections are the tables whose version is the project's own: Cargo's [package], PEP 621's
// [project], and Poetry's [tool.poetry]. Dependency tables also have "version" keys.
var tomlSections = map[string][]string{"Cargo.toml": {"package"}, "pyproject.toml": {"project", "tool.poetry"}}

// tomlVersionLine returns the index of the version line inside one of sections, or -1.
func tomlVersionLine(lines []string, sections []string) int {
	cur := ""
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			cur = strings.Trim(strings.TrimSpace(strings.SplitN(t, "#", 2)[0]), "[] ")
			continue
		}
		for _, sec := range sections {
			if cur == sec && tomlVersion.MatchString(t) {
				return i
			}
		}
	}
	return -1
}

// jsonTopLevelVersion locates the "version" value at the top level of a JSON object, so a
// package.json is edited in place without reformatting it.
func jsonTopLevelVersion(b []byte) (start, end int, ok bool) {
	depth, inStr, esc := 0, false, false
	keyStart := -1
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
				if depth == 1 && keyStart >= 0 && string(b[keyStart:i]) == "version" {
					j := i + 1
					for j < len(b) && (b[j] == ' ' || b[j] == '\t' || b[j] == '\n' || b[j] == '\r') {
						j++
					}
					if j < len(b) && b[j] == ':' {
						j++
						for j < len(b) && (b[j] == ' ' || b[j] == '\t' || b[j] == '\n' || b[j] == '\r') {
							j++
						}
						if j < len(b) && b[j] == '"' {
							k := j + 1
							for k < len(b) && b[k] != '"' {
								if b[k] == '\\' {
									k++
								}
								k++
							}
							return j, k + 1, k < len(b)
						}
					}
				}
				keyStart = -1
			}
			continue
		}
		switch c {
		case '"':
			inStr, keyStart = true, i+1
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
	return 0, 0, false
}

// manifests lists files whose version field aitk updates.
func manifests(p *project.Project) []string {
	var out []string
	if b, err := os.ReadFile(p.Path("package.json")); err == nil {
		if o, err := jsonedit.Parse(b); err == nil {
			if _, ok := o.Get("version"); ok {
				out = append(out, "package.json")
			}
		}
	}
	for _, f := range []string{"pyproject.toml", "Cargo.toml"} {
		if b, err := os.ReadFile(p.Path(f)); err == nil && tomlVersionLine(strings.Split(string(b), "\n"), tomlSections[f]) >= 0 {
			out = append(out, f)
		}
	}
	return out
}

// WriteManifests sets the version in each manifest.
func WriteManifests(p *project.Project, pl *Plan) error {
	for _, f := range pl.Manifests {
		b, err := os.ReadFile(p.Path(f))
		if err != nil {
			return err
		}
		var next []byte
		if f == "package.json" {
			i, j, ok := jsonTopLevelVersion(b)
			if !ok {
				return fmt.Errorf("package.json: no top-level version string")
			}
			next = append(append(append([]byte{}, b[:i]...), []byte(`"`+pl.Version+`"`)...), b[j:]...)
		} else {
			lines := strings.Split(string(b), "\n")
			i := tomlVersionLine(lines, tomlSections[f])
			if i < 0 {
				return fmt.Errorf("%s: no version in %s", f, strings.Join(tomlSections[f], " or "))
			}
			indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
			rest := tomlVersion.ReplaceAllString(strings.TrimSpace(lines[i]), `version = "`+pl.Version+`"`)
			lines[i] = indent + rest
			next = []byte(strings.Join(lines, "\n"))
		}
		if err := fsx.WriteFileKeep(p.Path(f), next, 0o644); err != nil {
			return err
		}
	}
	return nil
}
