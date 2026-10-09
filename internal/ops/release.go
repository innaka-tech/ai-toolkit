package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/v2/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/v2/internal/checkrun"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/profile"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/release"
	"github.com/innaka-tech/ai-toolkit/v2/internal/schema"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
)

// ReleaseInput controls aitk release.
type ReleaseInput struct {
	Bump      string
	Pre       string
	DryRun    bool
	Tag       bool // commit the release files and create an annotated tag
	SkipCheck bool
}

// ReleaseResult reports a release.
type ReleaseResult struct {
	*release.Plan
	Written  []string `json:"written,omitempty"`
	Commit   string   `json:"commit,omitempty"`
	Tagged   bool     `json:"tagged"`
	Warnings []string `json:"-"`
}

// ReleaseProject versions the project: SemVer from Conventional Commits, changelog, manifests, tag.
// It never pushes.
func ReleaseProject(p *project.Project, in ReleaseInput) (*ReleaseResult, error) {
	plan, err := release.Make(p, release.Options{Bump: in.Bump, Pre: in.Pre})
	if err != nil {
		code := "E_RELEASE"
		if strings.HasPrefix(err.Error(), "nothing to release") {
			code = "E_RELEASE_NOTHING"
		}
		return &ReleaseResult{Plan: plan}, apperr.New(code, apperr.ExitGate, "commit releasable work (feat, fix, perf) or pass --bump patch|minor|major", "%v", err)
	}
	res := &ReleaseResult{Plan: plan}
	// Consistency gates.
	if dirty := codeChanges(p); len(dirty) > 0 {
		return res, apperr.New("E_RELEASE_DIRTY", apperr.ExitGate, "commit or stash your changes, then release", "uncommitted changes: %s", strings.Join(dirty, ", "))
	}
	tasks, _ := task.List(p)
	for _, t := range tasks {
		switch t.Status {
		case task.InReview:
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s %q is in review (UAT or review pending) and is not part of a finished release", t.ID, t.Title))
		case task.InProgress:
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s %q is still in progress", t.ID, t.Title))
		}
	}
	if p.Config.Check.Cmd != "" && !in.SkipCheck && !in.DryRun {
		timeout, _ := time.ParseDuration(p.Config.CheckTimeout())
		if r := checkrun.Run(p.Root, p.Config.Check.Cmd, timeout, false); r.ExitCode != 0 {
			return res, apperr.New("E_RELEASE_CHECK", apperr.ExitGate, "fix the failing check before releasing", "the check fails (exit %d); a release must pass it", r.ExitCode)
		}
	}
	if in.DryRun {
		return res, nil
	}
	if err := release.WriteChangelog(p, plan); err != nil {
		return res, apperr.New("E_RELEASE", apperr.ExitGate, "check "+plan.Changelog, "%v", err)
	}
	res.Written = append(res.Written, plan.Changelog)
	if err := release.WriteManifests(p, plan); err != nil {
		return res, err
	}
	res.Written = append(res.Written, plan.Manifests...)
	if err := setStateVersion(p, plan.Version); err == nil {
		res.Written = append(res.Written, project.StateFile)
	}
	if !in.Tag {
		return res, nil
	}
	args := append([]string{"add", "--"}, res.Written...)
	if _, err := gitx.Run(p.Root, args...); err != nil {
		return res, apperr.New("E_GIT", apperr.ExitRuntime, "git status", "%v", err)
	}
	msg := "chore(release): " + plan.Tag
	if _, err := gitx.Run(p.Root, "commit", "-q", "-m", msg); err != nil {
		return res, apperr.New("E_GIT", apperr.ExitRuntime, "git status", "%v", err)
	}
	res.Commit = gitx.Head(p.Root)
	if _, err := gitx.Run(p.Root, "tag", "-a", plan.Tag, "-m", plan.Tag+"\n\n"+plan.Notes); err != nil {
		return res, apperr.New("E_GIT", apperr.ExitRuntime, "git tag", "%v", err)
	}
	res.Tagged = true
	return res, nil
}

// codeChanges lists uncommitted files other than aitk metadata.
func codeChanges(p *project.Project) []string {
	out, _ := gitx.RunRaw(p.Root, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	var files []string
	for _, rec := range strings.Split(string(out), "\x00") {
		if len(rec) > 3 && !profile.Managed(rec[3:]) {
			files = append(files, rec[3:])
		}
	}
	return files
}

func setStateVersion(p *project.Project, v string) error {
	path := p.Path(project.StateFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var st map[string]any
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	proj, _ := st["project"].(map[string]any)
	if proj == nil {
		return fmt.Errorf("ai-state.json has no project")
	}
	proj["version"] = v
	if err := schema.Validate("state", st); err != nil {
		return err
	}
	out, _ := json.MarshalIndent(st, "", "  ")
	return fsx.WriteFile(path, append(out, '\n'), 0o644)
}
