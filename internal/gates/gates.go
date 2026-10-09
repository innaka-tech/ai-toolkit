// Package gates implements the git hooks and CI gate (docs/spec/integrations.md, Gates).
package gates

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v3"

	"github.com/innaka-tech/ai-toolkit/v2/internal/agentsmd"
	"github.com/innaka-tech/ai-toolkit/v2/internal/doc"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/schema"
	"github.com/innaka-tech/ai-toolkit/v2/internal/secrets"
)

// Hooks managed by aitk.
var Hooks = []string{"pre-commit", "commit-msg", "pre-push"}

const (
	begin = "# aitk:begin"
	end   = "# aitk:end"
)

func block(hook string) string {
	return begin + "\n" +
		"# Installed by `aitk hooks install`; remove with `aitk hooks uninstall`.\n" +
		"if command -v aitk >/dev/null 2>&1; then aitk hook " + hook + ` "$@" || exit $?; fi` + "\n" +
		end + "\n"
}

// HooksDir returns the effective hooks directory (respects core.hooksPath).
func HooksDir(root string) (string, error) {
	return gitx.Run(root, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
}

// userSuffix holds a user's non-shell hook that aitk wraps.
const userSuffix = ".aitk-user"

var shellShebang = regexp.MustCompile(`^#!\s*(\S*/)?(env\s+)?(sh|bash|zsh|dash|ksh|ash)\b`)

func wrapper(hook string) string {
	return "#!/bin/sh\n" + block(hook) + `exec "$(dirname "$0")/` + hook + userSuffix + `" "$@"` + "\n"
}

// Install adds aitk's block to each hook. Shell hooks get the block after their shebang;
// hooks in other languages are kept as <hook>.aitk-user and run by a small shell wrapper,
// so existing hooks always keep working.
func Install(root string) ([]string, error) {
	dir, err := HooksDir(root)
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, h := range Hooks {
		path := filepath.Join(dir, h)
		b, _ := os.ReadFile(path)
		s := string(b)
		var next string
		switch i, j := strings.Index(s, begin), strings.Index(s, end); {
		case i >= 0 && j > i:
			next = s[:i] + block(h) + strings.TrimPrefix(s[j+len(end):], "\n")
		case strings.TrimSpace(s) == "":
			next = "#!/bin/sh\n" + block(h)
		case strings.HasPrefix(s, "#!") && !shellShebang.MatchString(s):
			user := path + userSuffix
			if fsx.Exists(user) {
				return changed, fmt.Errorf("%s already exists; remove it or merge it into %s", user, path)
			}
			if err := os.Rename(path, user); err != nil {
				return changed, err
			}
			next = wrapper(h)
		case strings.HasPrefix(s, "#!"):
			nl := strings.Index(s, "\n")
			if nl < 0 {
				next = s + "\n" + block(h)
			} else {
				next = s[:nl+1] + block(h) + s[nl+1:]
			}
		default:
			next = "#!/bin/sh\n" + block(h) + s
		}
		if next == s {
			continue
		}
		if err := fsx.WriteFileKeep(path, []byte(next), 0o755); err != nil {
			return changed, err
		}
		os.Chmod(path, 0o755)
		changed = append(changed, path)
	}
	return changed, nil
}

// Uninstall removes aitk's block; a wrapped hook is restored, and a hook left with only a
// shebang is deleted.
func Uninstall(root string) ([]string, error) {
	dir, err := HooksDir(root)
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, h := range Hooks {
		path := filepath.Join(dir, h)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		s := string(b)
		if s == wrapper(h) && fsx.Exists(path+userSuffix) {
			if err := os.Rename(path+userSuffix, path); err != nil {
				return changed, err
			}
			changed = append(changed, path)
			continue
		}
		i, j := strings.Index(s, begin), strings.Index(s, end)
		if i < 0 || j < i {
			continue
		}
		next := s[:i] + strings.TrimPrefix(s[j+len(end):], "\n")
		if strings.TrimSpace(next) == "#!/bin/sh" || strings.TrimSpace(next) == "" {
			os.Remove(path)
		} else if err := fsx.WriteFileKeep(path, []byte(next), 0o755); err != nil {
			return changed, err
		}
		changed = append(changed, path)
	}
	return changed, nil
}

// Violation is one gate failure.
type Violation struct {
	Gate   string `json:"gate"`
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Detail string `json:"detail"`
}

func (v Violation) String() string {
	loc := v.File
	if v.Line > 0 {
		loc = fmt.Sprintf("%s:%d", v.File, v.Line)
	}
	if loc != "" {
		loc += ": "
	}
	return fmt.Sprintf("[%s] %s%s", v.Gate, loc, v.Detail)
}

// PreCommit checks staged changes: secrets in added lines and validity of aitk files.
func PreCommit(root string) ([]Violation, error) {
	diff, err := gitx.Run(root, "diff", "--cached", "-U0", "--no-color", "--no-ext-diff", "--diff-filter=ACMR")
	if err != nil {
		return nil, err
	}
	vs := secretViolations(secrets.ScanDiff(diff))
	names, err := gitx.Run(root, "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	if err != nil {
		return nil, err
	}
	for _, rel := range strings.Split(names, "\n") {
		if rel == "" {
			continue
		}
		content, err := gitx.RunRaw(root, "show", ":"+rel)
		if err != nil {
			continue
		}
		vs = append(vs, validateFile(rel, content)...)
	}
	return vs, nil
}

func secretViolations(fs []secrets.Finding) []Violation {
	var vs []Violation
	for _, f := range fs {
		vs = append(vs, Violation{Gate: "secret", File: f.File, Line: f.Line, Detail: f.Rule + " (" + f.Match + "); remove it, rotate it, or mark a false positive with `aitk:allow-secret`"})
	}
	return vs
}

var (
	taskFile    = regexp.MustCompile(`^docs/ai/tasks/[^/]+\.md$`)
	handoffFile = regexp.MustCompile(`^docs/ai/handoff/[^/]+\.md$`)
)

// validateFile validates one aitk-managed file's content against its schema.
func validateFile(rel string, content []byte) []Violation {
	bad := func(d string) []Violation { return []Violation{{Gate: "schema", File: rel, Detail: d}} }
	switch {
	case rel == project.StateFile:
		var v any
		if err := json.Unmarshal(content, &v); err != nil {
			return bad("not valid JSON: " + err.Error())
		}
		if err := schema.Validate("state", v); err != nil {
			return bad(err.Error())
		}
	case rel == project.ConfigFile:
		var raw map[string]any
		if _, err := toml.Decode(string(content), &raw); err != nil {
			return bad("not valid TOML: " + err.Error())
		}
		if err := schema.Validate("config", raw); err != nil {
			return bad(err.Error())
		}
	case taskFile.MatchString(rel), handoffFile.MatchString(rel):
		front, _, ok := doc.Split(string(content))
		if !ok {
			return bad("missing YAML frontmatter")
		}
		var v map[string]any
		if err := yaml.Unmarshal([]byte(front), &v); err != nil {
			return bad("frontmatter: " + err.Error())
		}
		name := "task"
		if handoffFile.MatchString(rel) {
			name = "handoff"
		}
		if err := schema.Validate(name, normalizeYAML(v)); err != nil {
			return bad(err.Error())
		}
	case rel == project.AgentsFile:
		// A block from an older or newer aitk release is fine here (teammates may run different
		// versions); doctor offers the update. Only a missing or hand-edited block is rejected.
		if st := agentsmd.Inspect(string(content)); st == agentsmd.Missing || st == agentsmd.Edited || st == agentsmd.V1Only {
			return bad("aitk block missing or edited; run `aitk doctor --fix` and stage AGENTS.md")
		}
	}
	return nil
}

// normalizeYAML converts YAML-decoded values (time.Time for unquoted timestamps) into JSON-like ones.
func normalizeYAML(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			x[k] = normalizeYAML(val)
		}
		return x
	case []any:
		for i, val := range x {
			x[i] = normalizeYAML(val)
		}
		return x
	case time.Time:
		return x.UTC().Format("2006-01-02T15:04:05Z")
	}
	return v
}

// ConventionalRe matches a Conventional Commits 1.0 header.
var ConventionalRe = regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([^()\r\n]+\))?!?: \S.*$`)

// CheckMessage validates a commit message header. Merge, revert, fixup, and squash commits pass.
func CheckMessage(msg string) *Violation {
	var lines []string
	for _, l := range strings.Split(msg, "\n") {
		if !strings.HasPrefix(l, "#") {
			lines = append(lines, l)
		}
	}
	header := ""
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			header = strings.TrimSpace(l)
			break
		}
	}
	switch {
	case header == "":
		return nil // git aborts empty messages itself
	case strings.HasPrefix(header, "Merge "), strings.HasPrefix(header, "Revert \""), strings.HasPrefix(header, "fixup! "), strings.HasPrefix(header, "squash! "), strings.HasPrefix(header, "amend! "):
		return nil
	case !ConventionalRe.MatchString(header):
		return &Violation{Gate: "commit-message", Detail: fmt.Sprintf("%q is not a Conventional Commit; use `<type>(<scope>): <summary>` with type feat, fix, docs, style, refactor, perf, test, build, ci, chore, or revert", header)}
	case len([]rune(header)) > 100:
		return &Violation{Gate: "commit-message", Detail: "header longer than 100 characters"}
	}
	return nil
}

// AddTrailers appends AI-Task / AI-Tool trailers to the message file when not already present.
func AddTrailers(root, msgFile, taskID, tool string) error {
	var args []string
	if taskID != "" {
		args = append(args, "--trailer", "AI-Task: "+taskID)
	}
	if tool != "" && tool != "human" && tool != "unknown" {
		args = append(args, "--trailer", "AI-Tool: "+tool)
	}
	if len(args) == 0 {
		return nil
	}
	_, err := gitx.Run(root, append([]string{"interpret-trailers", "--in-place", "--if-exists", "addIfDifferent"}, append(args, msgFile)...)...)
	return err
}

// CI runs the gates over base..HEAD: commit messages and secrets in the diff.
func CI(root, base string) ([]Violation, error) {
	var vs []Violation
	if base == "" {
		return vs, nil
	}
	diff, err := gitx.Run(root, "diff", "-U0", "--no-color", "--no-ext-diff", "--diff-filter=ACMR", base+"...HEAD")
	if err != nil {
		return nil, err
	}
	vs = append(vs, secretViolations(secrets.ScanDiff(diff))...)
	log, err := gitx.Run(root, "log", "--no-merges", "--format=%H%x00%B%x1e", base+"..HEAD")
	if err != nil {
		return nil, err
	}
	for _, rec := range strings.Split(log, "\x1e") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		hash, body, _ := strings.Cut(rec, "\x00")
		if v := CheckMessage(body); v != nil {
			v.File = hash[:min(7, len(hash))]
			vs = append(vs, *v)
		}
	}
	names, _ := gitx.Run(root, "diff", "--name-only", "--diff-filter=ACMR", base+"...HEAD")
	for _, rel := range strings.Split(names, "\n") {
		if rel == "" {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(root, rel)); err == nil {
			vs = append(vs, validateFile(rel, b)...)
		}
	}
	return vs, nil
}

// DefaultBase picks the CI comparison base: GITHUB_BASE_REF / CI_MERGE_REQUEST_TARGET_BRANCH_NAME,
// else the merge base with origin/<default branch>, else "" (nothing to compare).
func DefaultBase(root, defaultBranch string) string {
	for _, env := range []string{"GITHUB_BASE_REF", "CI_MERGE_REQUEST_TARGET_BRANCH_NAME"} {
		if b := os.Getenv(env); b != "" {
			if _, err := gitx.Run(root, "rev-parse", "--verify", "-q", "origin/"+b); err == nil {
				return "origin/" + b
			}
			return b
		}
	}
	for _, ref := range []string{"origin/" + defaultBranch, defaultBranch} {
		mb, err := gitx.Run(root, "merge-base", ref, "HEAD")
		if err == nil && mb != "" && mb != gitx.Head(root) {
			return mb
		}
	}
	return ""
}
