// Package conv reads the project's coding conventions (docs/ai/conventions.md).
package conv

import (
	"os"
	"regexp"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
)

// File holds the project's coding conventions, shown to every agent in the brief.
const File = "docs/ai/conventions.md"

// Template is written by init. Lines with "<…>" are placeholders.
const Template = `# Conventions

<!-- Every agent sees the first lines of this file in its brief. Keep it short and specific. -->

- Stack and versions: <language, framework, database, and their versions>
- Structure: <where code, tests, and migrations live; one module per …>
- Naming and style: <formatter/linter and its config; naming rules>
- Errors: <how errors are returned, logged, and shown to users>
- Data: <money as decimal strings; times stored in UTC, shown in local time; IDs …>
- Tests: <test framework; what needs a test; how to run them>
- Security: <auth model; input validation; secrets only in environment variables>
- Git and versioning: Conventional Commits; aitk release for versions and the changelog
- UI consistency: <design tokens/components to reuse; patterns to avoid>
`

var placeholder = regexp.MustCompile(`<[^>]+>`)

// Lines returns up to n meaningful lines, and whether the file is still the template.
func Lines(p *project.Project, n int) (lines []string, unfilled bool, exists bool) {
	b, err := os.ReadFile(p.Path(File))
	if err != nil {
		return nil, false, false
	}
	inComment := false
	filled := 0
	for _, l := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "<!--"):
			inComment = !strings.Contains(t, "-->")
			continue
		case inComment:
			inComment = !strings.Contains(t, "-->")
			continue
		case t == "" || strings.HasPrefix(t, "# "):
			continue
		}
		if placeholder.MatchString(t) && !strings.Contains(t, "`") {
			continue // template placeholder not filled in yet
		}
		filled++
		if len(lines) < n {
			lines = append(lines, t)
		}
	}
	return lines, filled <= 1, true
}
