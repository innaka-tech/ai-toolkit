package compat

import (
	"os"
	"strings"

	"github.com/innaka-tech/ai-toolkit/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/internal/project"
)

const (
	gaBegin = "# aitk:begin"
	gaEnd   = "# aitk:end"
)

// gitattributes lets parallel branches merge without conflicts: knowledge files are
// append-only lists (union merge keeps both sides), and generated indexes are rebuilt
// by the next aitk command.
const gitattributes = gaBegin + `
docs/ai/knowledge/*.md merge=union
docs/ai/knowledge/_archive/*.md merge=union
docs/ai/handoff.md merge=union linguist-generated
docs/ai/current-task.md merge=union linguist-generated
docs/ai/knowledge.md merge=union linguist-generated
docs/ai/decisions.md merge=union linguist-generated
` + gaEnd + "\n"

// EnsureGitattributes installs aitk's block in .gitattributes. It reports whether the file changed.
func EnsureGitattributes(p *project.Project, dryRun bool) (bool, error) {
	path := p.Path(".gitattributes")
	b, _ := os.ReadFile(path)
	s := string(b)
	var next string
	if i, j := strings.Index(s, gaBegin), strings.Index(s, gaEnd); i >= 0 && j > i {
		next = s[:i] + gitattributes + strings.TrimPrefix(s[j+len(gaEnd):], "\n")
	} else {
		if s != "" && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		next = s + gitattributes
	}
	if next == string(b) {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	return true, fsx.WriteFile(path, []byte(next), 0o644)
}
