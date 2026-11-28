package ops

import (
	"github.com/innaka-tech/ai-toolkit/v2/internal/profile"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/slop"
)

// Slop scans the given paths, or else the files changed on this branch (aitk metadata excluded).
func Slop(p *project.Project, paths []string) []slop.Hit {
	if len(paths) == 0 {
		for _, c := range profile.Diff(p) {
			paths = append(paths, c.Path)
		}
	}
	return slop.Scan(p, paths)
}

// TrackedFiles lists tracked and untracked (not ignored) files, for a whole-repository scan.
func TrackedFiles(p *project.Project) []string { return trackedFiles(p) }
