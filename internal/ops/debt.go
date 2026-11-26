package ops

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
)

// Shortcut is one deliberate limit recorded in code as "shortcut: <the limit>, <when to upgrade>".
type Shortcut struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// shortcutRE matches the marker only after a comment opener, so prose that mentions the
// convention (markdown, strings) is not counted.
var shortcutRE = regexp.MustCompile(`(?:^|\s)(?://|#|--|/\*|<!--|;)\s*shortcut:\s*(.*?)\s*(?:\*/|-->)?\s*$`)

// Debt lists the shortcut comments in tracked and untracked (not ignored) files.
func Debt(p *project.Project) []Shortcut {
	out, _ := gitx.RunRaw(p.Root, "grep", "--untracked", "--full-name", "-I", "-n", "-z", "-i", "-e", "shortcut:",
		"--", ".", ":(exclude)*.md", ":(exclude)docs/ai", ":(exclude)vendor", ":(exclude)node_modules")
	var res []Shortcut
	for _, rec := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(rec, "\x00", 3)
		if len(parts) != 3 {
			continue
		}
		m := shortcutRE.FindStringSubmatch(parts[2])
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(parts[1])
		res = append(res, Shortcut{File: parts[0], Line: n, Text: m[1]})
	}
	return res
}
