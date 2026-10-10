package adapters

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/slash"
)

// commandChanges writes cmds into dir (under base: the repository or the home directory) in a
// tool's format, and removes command files aitk generated there earlier that are no longer
// wanted. A file of the user's with the same name is left alone and reported.
func commandChanges(base, dir, display string, f slash.Format, cmds []slash.Command, global, remove bool) []Change {
	var out []Change
	abs := filepath.Join(base, filepath.FromSlash(dir))
	want := map[string]bool{}
	if !remove {
		for _, c := range cmds {
			name := c.Name + f.Ext
			want[name] = true
			p := filepath.Join(abs, name)
			before := read(p)
			after := f.Render(c)
			ch := Change{Path: p, Rel: display + "/" + name, What: "slash command /" + c.Name, Global: global, before: before, after: after}
			switch {
			case isLink(p):
				ch.problem, ch.after, ch.note = "a symlink; aitk leaves linked commands to you", before, true
			case len(before) > 0 && !slash.Generated(before):
				ch.problem, ch.after, ch.note = "a command of yours with this name exists; left alone", before, true
			case f.Name == "windsurf" && len(after) > 12000:
				ch.problem, ch.after = "longer than Windsurf's 12,000-character workflow limit", before
			}
			out = append(out, ch)
		}
	}
	entries, _ := os.ReadDir(abs)
	var stale []string
	for _, e := range entries {
		if e.IsDir() || want[e.Name()] || !strings.HasSuffix(e.Name(), f.Ext) {
			continue
		}
		p := filepath.Join(abs, e.Name())
		if b := read(p); slash.Generated(b) && !isLink(p) {
			stale = append(stale, e.Name())
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		p := filepath.Join(abs, name)
		out = append(out, Change{Path: p, Rel: display + "/" + name, What: "remove slash command /" + strings.TrimSuffix(name, f.Ext), Global: global, before: read(p), Delete: true})
	}
	return out
}

func isLink(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// projectCommands are the repository's own commands (docs/ai/commands).
func projectCommands(e Env) []slash.Command {
	cs, _ := slash.Project(e.Root)
	return cs
}

// commandWarnings reports project command files that could not be used, once per sync.
func commandWarnings(e Env) []Change {
	_, warns := slash.Project(e.Root)
	var out []Change
	for _, w := range warns {
		out = append(out, Change{Rel: slash.ProjectDir, What: "project command", problem: w})
	}
	return out
}

// allCommands are the built-in commands plus the repository's own, for tools that have no
// user-level command folder (their built-ins cannot come from aitk setup).
func allCommands(e Env) []slash.Command {
	cs, _ := slash.All(e.Root)
	return cs
}

func projectCmds(e Env, dir, format string, cmds []slash.Command) []Change {
	return commandChanges(e.Root, dir, dir, slash.Formats[format], cmds, false, false)
}
