// Package project locates an aitk project and loads its configuration.
package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/innaka-tech/ai-toolkit/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/internal/schema"
)

// Config mirrors schemas/config.schema.json.
type Config struct {
	Project   ProjectCfg   `toml:"project,omitempty" json:"project,omitempty"`
	Check     CheckCfg     `toml:"check,omitempty" json:"check,omitempty"`
	Risk      RiskCfg      `toml:"risk,omitempty" json:"risk,omitempty"`
	Brief     BriefCfg     `toml:"brief,omitempty" json:"brief,omitempty"`
	Knowledge KnowledgeCfg `toml:"knowledge,omitempty" json:"knowledge,omitempty"`
	Deploy    DeployCfg    `toml:"deploy,omitempty" json:"deploy,omitempty"`
	Plugins   PluginsCfg   `toml:"plugins,omitempty" json:"plugins,omitempty"`
}

type ProjectCfg struct {
	Name          string `toml:"name,omitempty" json:"name,omitempty"`
	DefaultBranch string `toml:"default_branch,omitempty" json:"default_branch,omitempty"`
}
type CheckCfg struct {
	Cmd     string `toml:"cmd,omitempty" json:"cmd,omitempty"`
	Timeout string `toml:"timeout,omitempty" json:"timeout,omitempty"`
}
type RiskCfg struct {
	SensitivePaths []string `toml:"sensitive_paths,omitempty" json:"sensitive_paths,omitempty"`
	LiteMaxFiles   *int     `toml:"lite_max_files,omitempty" json:"lite_max_files,omitempty"`
	LiteMaxLines   *int     `toml:"lite_max_lines,omitempty" json:"lite_max_lines,omitempty"`
}
type BriefCfg struct {
	Budget int `toml:"budget,omitempty" json:"budget,omitempty"`
}
type KnowledgeCfg struct {
	InboxLimit       int `toml:"inbox_limit,omitempty" json:"inbox_limit,omitempty"`
	ArchiveAfterDays int `toml:"archive_after_days,omitempty" json:"archive_after_days,omitempty"`
}
type DeployCfg struct {
	Targets   map[string]string `toml:"targets,omitempty" json:"targets,omitempty"`
	PostCheck string            `toml:"post_check,omitempty" json:"post_check,omitempty"`
}
type PluginsCfg struct {
	Enabled  []string                  `toml:"enabled,omitempty" json:"enabled,omitempty"`
	Timeout  string                    `toml:"timeout,omitempty" json:"timeout,omitempty"`
	Settings map[string]map[string]any `toml:"settings,omitempty" json:"settings,omitempty"`
}

// Effective defaults (docs/spec/workflow.md, schemas/config.schema.json).
func (c Config) LiteMaxFiles() int {
	if c.Risk.LiteMaxFiles != nil {
		return *c.Risk.LiteMaxFiles
	}
	return 3
}
func (c Config) LiteMaxLines() int {
	if c.Risk.LiteMaxLines != nil {
		return *c.Risk.LiteMaxLines
	}
	return 100
}
func (c Config) BriefBudget() int {
	if c.Brief.Budget > 0 {
		return c.Brief.Budget
	}
	return 4000
}
func (c Config) InboxLimit() int {
	if c.Knowledge.InboxLimit > 0 {
		return c.Knowledge.InboxLimit
	}
	return 20
}
func (c Config) ArchiveAfterDays() int {
	if c.Knowledge.ArchiveAfterDays > 0 {
		return c.Knowledge.ArchiveAfterDays
	}
	return 90
}
func (c Config) CheckTimeout() string {
	if c.Check.Timeout != "" {
		return c.Check.Timeout
	}
	return "10m"
}
func (c Config) DefaultBranch() string {
	if c.Project.DefaultBranch != "" {
		return c.Project.DefaultBranch
	}
	return "main"
}

// Project is a discovered repository.
type Project struct {
	Root      string // git toplevel
	GitDir    string // per-worktree git dir
	CommonDir string // shared git dir
	Config    Config
}

// Paths relative to Root.
const (
	ConfigFile  = "aitk.toml"
	StateFile   = "ai-state.json"
	AgentsFile  = "AGENTS.md"
	DocsAI      = "docs/ai"
	TasksDir    = "docs/ai/tasks"
	HandoffDir  = "docs/ai/handoff"
	KnowDir     = "docs/ai/knowledge"
	LegacyDir   = "docs/ai/_legacy"
	ADRDir      = "docs/adr"
	ContextFile = "docs/ai/project-context.md"
)

// Path joins rel onto the project root.
func (p *Project) Path(rel ...string) string {
	return filepath.Join(append([]string{p.Root}, rel...)...)
}

// LocalStateDir is the in-tree fallback for aitk's private state, used when the git
// directory is read-only (for example inside an agent sandbox). It ignores itself.
const LocalStateDir = ".aitk"

// StateDirs lists per-worktree state locations in order of preference.
func (p *Project) StateDirs() []string {
	return []string{filepath.Join(p.GitDir, "aitk"), filepath.Join(p.Root, LocalStateDir)}
}

// SharedStateDirs lists locations shared by all worktrees, in order of preference.
func (p *Project) SharedStateDirs() []string {
	return []string{filepath.Join(p.CommonDir, "aitk"), filepath.Join(p.Root, LocalStateDir)}
}

// AitkDir is the per-worktree private state directory (the first writable of StateDirs).
func (p *Project) AitkDir() string { return writableDir(p.StateDirs()) }

// SharedDir is the shared state directory (the first writable of SharedStateDirs).
func (p *Project) SharedDir() string { return writableDir(p.SharedStateDirs()) }

// LockPath is the project lock, shared by all worktrees when the git directory is writable.
func (p *Project) LockPath() string { return filepath.Join(p.SharedDir(), "lock") }

var (
	writableMu    sync.Mutex
	writableCache = map[string]bool{}
)

// writableDir returns the first directory that can be created and written to.
// When none can, it returns the first one (callers then report the write error).
func writableDir(dirs []string) string {
	writableMu.Lock()
	defer writableMu.Unlock()
	for _, d := range dirs {
		ok, seen := writableCache[d]
		if !seen {
			ok = probe(d)
			writableCache[d] = ok
		}
		if ok {
			return d
		}
	}
	return dirs[0]
}

func probe(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	if filepath.Base(dir) == LocalStateDir {
		ign := filepath.Join(dir, ".gitignore")
		if _, err := os.Stat(ign); err != nil {
			os.WriteFile(ign, []byte("# aitk private state (fallback when .git is read-only)\n*\n"), 0o644)
		}
	}
	return true
}

// Repo locates the git repository containing dir without requiring aitk.toml.
func Repo(dir string) (*Project, error) {
	root, err := gitx.Toplevel(dir)
	if err != nil {
		return nil, apperr.New("E_NOT_A_PROJECT", apperr.ExitGate, "git init", "not inside a git repository")
	}
	gd, err := gitx.GitDir(root)
	if err != nil {
		return nil, err
	}
	cd, err := gitx.CommonDir(root)
	if err != nil {
		return nil, err
	}
	return &Project{Root: root, GitDir: gd, CommonDir: cd}, nil
}

// Open locates the project and loads aitk.toml. A v1 project yields E_NEEDS_MIGRATION.
func Open(dir string) (*Project, error) {
	p, err := Repo(dir)
	if err != nil {
		return nil, err
	}
	if !exists(p.Path(ConfigFile)) {
		if IsV1(p.Root) {
			return nil, apperr.NeedsMigration()
		}
		return nil, apperr.NotAProject()
	}
	cfg, err := LoadConfig(p.Path(ConfigFile))
	if err != nil {
		return nil, apperr.New("E_SCHEMA", apperr.ExitGate, "fix aitk.toml (see schemas/config.schema.json)", "aitk.toml: %v", err)
	}
	if cfg.Project.Name == "" {
		cfg.Project.Name = Slug(filepath.Base(p.Root), 62)
	}
	p.Config = cfg
	return p, nil
}

// LoadConfig parses aitk.toml, rejecting unknown keys and values that violate
// schemas/config.schema.json.
func LoadConfig(path string) (Config, error) {
	var c Config
	md, err := toml.DecodeFile(path, &c)
	if err != nil {
		return c, err
	}
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return c, err
	}
	if err := schema.Validate("config", raw); err != nil {
		return c, err
	}
	if und := md.Undecoded(); len(und) > 0 {
		keys := make([]string, 0, len(und))
		for _, k := range und {
			if len(k) >= 2 && k[0] == "plugins" && k[1] == "settings" {
				continue // free-form per-plugin settings
			}
			keys = append(keys, k.String())
		}
		if len(keys) > 0 {
			return c, &unknownKeys{keys}
		}
	}
	return c, nil
}

type unknownKeys struct{ keys []string }

func (u *unknownKeys) Error() string { return "unknown keys: " + strings.Join(u.keys, ", ") }

// IsV1 reports whether root holds a v1 ai-toolkit project.
func IsV1(root string) bool {
	if exists(filepath.Join(root, ConfigFile)) {
		return false
	}
	if exists(filepath.Join(root, ".ai-toolkit", "project.env")) {
		return true
	}
	b, err := os.ReadFile(filepath.Join(root, StateFile))
	if err != nil {
		return false
	}
	var s map[string]any
	if json.Unmarshal(b, &s) != nil {
		return false
	}
	_, v2 := s["schema_version"]
	return !v2
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slug lowercases and hyphenates text, capped at maxLen.
func Slug(s string, maxLen int) string {
	out := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(out) > maxLen {
		out = strings.TrimRight(out[:maxLen], "-")
	}
	if out == "" {
		return "untitled"
	}
	return out
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// DetectCheck guesses a check command from common build files.
func DetectCheck(root string) string {
	has := func(n string) bool { return exists(filepath.Join(root, n)) }
	switch {
	case has("go.mod"):
		return "go vet ./... && go test ./..."
	case has("Cargo.toml"):
		return "cargo test"
	case has("package.json"):
		b, _ := os.ReadFile(filepath.Join(root, "package.json"))
		var pj struct {
			Scripts map[string]string `json:"scripts"`
		}
		json.Unmarshal(b, &pj)
		if _, ok := pj.Scripts["test"]; ok {
			switch {
			case has("pnpm-lock.yaml"):
				return "pnpm test"
			case has("yarn.lock"):
				return "yarn test"
			case has("bun.lockb") || has("bun.lock"):
				return "bun run test"
			}
			return "npm test"
		}
	case has("pyproject.toml") || has("pytest.ini"):
		return "pytest"
	case has("composer.json"):
		return "composer test"
	case has("Makefile"):
		b, _ := os.ReadFile(filepath.Join(root, "Makefile"))
		if strings.Contains(string(b), "\ntest:") || strings.HasPrefix(string(b), "test:") {
			return "make test"
		}
	}
	return ""
}
