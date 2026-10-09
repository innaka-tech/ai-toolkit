// Package agentsmd maintains aitk's block in AGENTS.md (docs/spec/workflow.md §2).
package agentsmd

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
)

const (
	Begin   = "<!-- aitk:begin v=2 -->"
	End     = "<!-- aitk:end -->"
	v1Begin = "<!-- ai-toolkit:protocol:start -->"
	v1End   = "<!-- ai-toolkit:protocol:end -->"
)

// Block is the canonical protocol block.
const Block = Begin + `
<!-- aitk:block-revision 2 -->
## Working in this repository (aitk)

1. Run ` + "`aitk brief`" + ` and read its output. Do not read other files under docs/ai/ unless the brief points to them.
2. Run ` + "`aitk task next --start`" + ` (or ` + "`aitk task start <id>`" + `) to take a task, or ` + "`aitk task new \"<title>\"`" + ` to create one.
3. Do the work. Run ` + "`aitk check`" + ` until it passes, and ` + "`aitk impact`" + ` to see what your change can break.
4. Run ` + "`aitk close --summary \"<what changed>\" --knowledge \"<lasting finding, or none>\"`" + `.
5. If unsure, blocked, the change is risky, or the check fails twice: stop and ask the user.

If ` + "`aitk`" + ` is not installed, read docs/ai/project-context.md and the newest file in docs/ai/handoff/.
` + End

// State of the block in a file.
type State int

const (
	Missing State = iota
	Current
	Outdated
	V1Only
	Edited // a v2 block that matches no aitk release: changed by hand
	Newer  // the block of a later aitk release (higher block revision)
)

// Revision of Block. Later releases raise it, so an older aitk recognizes their block as newer
// rather than hand-edited, and never replaces it with its own.
const Revision = 2

var revisionLine = regexp.MustCompile(`<!-- aitk:block-revision (\d+) -->`)

// previous holds the blocks of earlier aitk releases. A file with one of them is Outdated, not
// Edited, so teammates on different aitk versions do not reject each other's commits.
var previous = []string{Begin + `
## Working in this repository (aitk)

1. Run ` + "`aitk brief`" + ` and read its output. Do not read other files under docs/ai/ unless the brief points to them.
2. Run ` + "`aitk task start <id>`" + ` to continue a task, or ` + "`aitk task new \"<title>\"`" + ` to create one.
3. Do the work. Run ` + "`aitk check`" + ` until it passes.
4. Run ` + "`aitk close --summary \"<what changed>\" --knowledge \"<lasting finding, or none>\"`" + `.
5. If unsure, blocked, the change is risky, or the check fails twice: stop and ask the user.

If ` + "`aitk`" + ` is not installed, read docs/ai/project-context.md and the newest file in docs/ai/handoff/.
` + End}

// Inspect reports the block state of content.
func Inspect(content string) State {
	content = strings.ReplaceAll(content, "\r\n", "\n") // CRLF checkouts
	i, j := strings.Index(content, Begin), strings.Index(content, End)
	if i >= 0 && j > i {
		got := content[i : j+len(End)]
		if got == Block {
			return Current
		}
		for _, old := range previous {
			if got == old {
				return Outdated
			}
		}
		if m := revisionLine.FindStringSubmatch(got); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > Revision {
				return Newer
			}
		}
		return Edited
	}
	if strings.Contains(content, v1Begin) {
		return V1Only
	}
	return Missing
}

// Apply returns content with the block installed: replacing a v2 block, else a v1 block,
// else appending. Text outside the block is preserved byte for byte.
func Apply(content string) string {
	for _, pair := range [][2]string{{Begin, End}, {v1Begin, v1End}} {
		i := strings.Index(content, pair[0])
		j := strings.Index(content, pair[1])
		if i >= 0 && j > i {
			return content[:i] + Block + content[j+len(pair[1]):]
		}
	}
	if strings.TrimSpace(content) == "" {
		return "# AGENTS.md\n\n" + Block + "\n"
	}
	sep := "\n\n"
	if strings.HasSuffix(content, "\n\n") {
		sep = ""
	} else if strings.HasSuffix(content, "\n") {
		sep = "\n"
	}
	return content + sep + Block + "\n"
}

// Sync applies the block to the file at path. It reports whether the file changed.
func Sync(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if Inspect(string(b)) == Newer {
		return false, nil // never downgrade a newer release's block
	}
	next := Apply(string(b))
	if next == string(b) {
		return false, nil
	}
	return true, fsx.WriteFileKeep(path, []byte(next), 0o644)
}
