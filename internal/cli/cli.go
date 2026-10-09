// Package cli wires aitk's commands (docs/spec/cli.md).
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/innaka-tech/ai-toolkit/internal/adapters"
	"github.com/innaka-tech/ai-toolkit/internal/adr"
	"github.com/innaka-tech/ai-toolkit/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/internal/brief"
	"github.com/innaka-tech/ai-toolkit/internal/checkrun"
	"github.com/innaka-tech/ai-toolkit/internal/compat"
	"github.com/innaka-tech/ai-toolkit/internal/doctor"
	"github.com/innaka-tech/ai-toolkit/internal/gates"
	"github.com/innaka-tech/ai-toolkit/internal/handoff"
	"github.com/innaka-tech/ai-toolkit/internal/knowledge"
	"github.com/innaka-tech/ai-toolkit/internal/mcpserver"
	"github.com/innaka-tech/ai-toolkit/internal/migrate"
	"github.com/innaka-tech/ai-toolkit/internal/ops"
	"github.com/innaka-tech/ai-toolkit/internal/plugins"
	"github.com/innaka-tech/ai-toolkit/internal/project"
	"github.com/innaka-tech/ai-toolkit/internal/schema"
	"github.com/innaka-tech/ai-toolkit/internal/session"
	"github.com/innaka-tech/ai-toolkit/internal/task"
	"github.com/innaka-tech/ai-toolkit/internal/textx"
)

// Build information, set with -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// result is what every command produces.
type result struct {
	data     any
	human    string
	warnings []string
}

type runFn func(cmd *cobra.Command, args []string) (*result, error)

type app struct {
	json   bool
	dir    string
	stdout io.Writer
	stderr io.Writer
	code   int
}

// Execute runs the CLI and returns the process exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	a := &app{stdout: stdout, stderr: stderr}
	root := a.root()
	for _, arg := range args {
		if arg == "--json" {
			a.json = true // so parse errors are reported as JSON too
		}
	}
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		// cobra-level errors: unknown command/flag
		a.fail(commandPath(root, args), apperr.Usage("%s", err.Error()))
	}
	return a.code
}

func commandPath(root *cobra.Command, args []string) string {
	if c, _, err := root.Find(args); err == nil && c != nil {
		return strings.TrimPrefix(c.CommandPath(), "aitk ")
	}
	return "aitk"
}

func (a *app) wrap(fn runFn) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		name := strings.TrimPrefix(cmd.CommandPath(), "aitk ")
		res, err := fn(cmd, args)
		if err != nil {
			a.fail(name, err, res)
			return nil
		}
		a.ok(name, res)
		return nil
	}
}

func (a *app) ok(name string, r *result) {
	if r == nil {
		r = &result{}
	}
	if a.json {
		a.emit(map[string]any{"schema": "aitk.result/v1", "ok": true, "command": name, "data": r.data, "warnings": nonNil(r.warnings)})
		return
	}
	for _, w := range r.warnings {
		fmt.Fprintln(a.stderr, "warning: "+w)
	}
	if r.human != "" {
		fmt.Fprint(a.stdout, strings.TrimRight(r.human, "\n")+"\n")
	}
}

func (a *app) fail(name string, err error, partial ...*result) {
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		ae = &apperr.Error{Code: "E_RUNTIME", Message: err.Error(), Exit: apperr.ExitRuntime}
	}
	a.code = ae.Exit
	var data any
	if len(partial) > 0 && partial[0] != nil {
		data = partial[0].data
	}
	if a.json {
		e := map[string]any{"code": ae.Code, "message": ae.Message}
		if ae.Fix != "" {
			e["fix"] = ae.Fix
		}
		a.emit(map[string]any{"schema": "aitk.result/v1", "ok": false, "command": name, "data": data, "warnings": []string{}, "error": e})
		return
	}
	if len(partial) > 0 && partial[0] != nil && partial[0].human != "" {
		fmt.Fprint(a.stdout, strings.TrimRight(partial[0].human, "\n")+"\n")
	}
	fmt.Fprintf(a.stderr, "error[%s]: %s\n", ae.Code, ae.Message)
	if ae.Fix != "" {
		fmt.Fprintf(a.stderr, "fix: %s\n", ae.Fix)
	}
}

func (a *app) emit(v map[string]any) {
	if v["data"] == nil {
		delete(v, "data")
	}
	if err := schema.Validate("result", v); err != nil {
		v = map[string]any{"schema": "aitk.result/v1", "ok": false, "command": v["command"], "error": map[string]any{"code": "E_INTERNAL", "message": "invalid result envelope: " + err.Error()}}
		a.code = apperr.ExitRuntime
	}
	enc := json.NewEncoder(a.stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (a *app) open() (*project.Project, error) {
	dir := a.dir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return project.Open(dir)
}

func (a *app) root() *cobra.Command {
	root := &cobra.Command{
		Use:           "aitk",
		Short:         "Continuity and quality layer for AI coding agents",
		Long:          "aitk keeps project state in git so any AI agent can resume another's work, and verifies every task before it is done.\nSpec: https://github.com/innaka-tech/ai-toolkit/tree/main/docs/spec",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(&a.json, "json", false, "print one aitk.result/v1 JSON object")
	root.PersistentFlags().StringVarP(&a.dir, "path", "C", "", "run as if started in this directory")
	root.CompletionOptions.HiddenDefaultCmd = true
	root.AddCommand(a.initCmd(), a.migrateCmd(), a.doctorCmd(), a.briefCmd(), a.taskCmd(), a.checkCmd(), a.closeCmd(),
		a.reviewCmd(), a.knowledgeCmd(), a.adrCmd(), a.versionCmd(), a.mcpCmd(), a.adaptersCmd(), a.hooksCmd(), a.hookCmd(), a.ciCmd(), a.workCmd(), a.switchCmd(), a.reportCmd(), a.logCmd(), a.pluginCmd(), a.importCmd(), a.deployCmd())
	return root
}

// ---------- project ----------

func (a *app) initCmd() *cobra.Command {
	var name, check string
	var dry bool
	c := &cobra.Command{Use: "init", Short: "Create aitk.toml, the AGENTS.md block, ai-state.json, and docs/ai/ (idempotent)", Args: cobra.NoArgs}
	c.Flags().StringVar(&name, "name", "", "project name (default: directory name)")
	c.Flags().StringVar(&check, "check", "", "check command, e.g. \"make test\" (default: detected)")
	c.Flags().BoolVar(&dry, "dry-run", false, "show what would be created")
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		dir := a.dir
		if dir == "" {
			dir, _ = os.Getwd()
		}
		r, err := ops.Init(dir, name, check, dry)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, f := range r.Created {
			b.WriteString("created " + f + "\n")
		}
		for _, f := range r.Updated {
			b.WriteString("updated " + f + "\n")
		}
		if len(r.Created)+len(r.Updated) == 0 {
			b.WriteString("already initialized; nothing to do\n")
		}
		var warns []string
		if r.Check == "" {
			warns = append(warns, `no check command detected; set [check] cmd in aitk.toml so close can verify work`)
		} else {
			b.WriteString("check: " + r.Check + "\n")
		}
		b.WriteString("next: aitk brief")
		return &result{data: r, human: b.String(), warnings: warns}, nil
	})
	return c
}

func (a *app) migrateCmd() *cobra.Command {
	var dry bool
	c := &cobra.Command{Use: "migrate", Short: "Convert a v1 ai-toolkit project to v2 (lossless; originals kept in docs/ai/_legacy/)", Args: cobra.NoArgs}
	c.Flags().BoolVar(&dry, "dry-run", false, "write nothing; print the report")
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		dir := a.dir
		if dir == "" {
			dir, _ = os.Getwd()
		}
		r, err := migrate.Run(dir, dry)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		switch {
		case r.AlreadyV2:
			b.WriteString("already a v2 project; nothing to do")
		case len(r.Counts) == 0 && len(r.Written) == 0:
			b.WriteString("nothing to migrate")
		default:
			verb := "migrated"
			if dry {
				verb = "would migrate"
			}
			fmt.Fprintf(&b, "%s %s: %d tasks, %d handoffs, %d knowledge entries, %d decisions (v1 context ≈ %d tokens)\n", verb, r.Project,
				r.Counts["tasks"], r.Counts["handoffs"], r.Counts["knowledge"], r.Counts["adrs"], r.V1Tokens)
			fmt.Fprintf(&b, "%d files written, %d originals preserved in docs/ai/_legacy/\n", len(r.Written), len(r.Legacy))
			if dry {
				b.WriteString("run without --dry-run to apply")
			} else {
				b.WriteString("next: review the diff (git diff --stat), then: aitk doctor && aitk brief")
			}
		}
		return &result{data: r, human: b.String(), warnings: r.Warnings}, nil
	})
	return c
}

func (a *app) doctorCmd() *cobra.Command {
	var fix bool
	c := &cobra.Command{Use: "doctor", Short: "Validate the project; --fix repairs what is safe", Args: cobra.NoArgs}
	c.Flags().BoolVar(&fix, "fix", false, "repair generated files, the AGENTS.md block, and an overfull inbox")
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		var r *doctor.Report
		err = ops.WithLock(p, func() error { r = doctor.Run(p, fix); return nil })
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, c := range r.Checks {
			mark := map[string]string{"ok": "ok   ", "warn": "WARN ", "error": "ERROR"}[c.Status]
			line := mark + " " + c.Name
			if c.Detail != "" {
				line += ": " + c.Detail
			}
			if c.Fixed {
				line += " (fixed)"
			}
			b.WriteString(line + "\n")
		}
		fmt.Fprintf(&b, "%d errors, %d warnings", r.Errors, r.Warns)
		res := &result{data: r, human: b.String()}
		if r.Errors > 0 {
			return res, apperr.New("E_SCHEMA", apperr.ExitGate, "aitk doctor --fix (or fix the files listed above)", "%d problem(s) found", r.Errors)
		}
		return res, nil
	})
	return c
}

func (a *app) briefCmd() *cobra.Command {
	var budget int
	var id string
	c := &cobra.Command{Use: "brief", Short: "Print the bounded session brief: start every session here", Args: cobra.NoArgs}
	c.Flags().IntVar(&budget, "budget", 0, "token budget (default: brief.budget or 4000)")
	c.Flags().StringVar(&id, "task", "", "brief for this task instead of the active one")
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		b := brief.Build(p, budget, id)
		var warns []string
		warns = append(warns, b.PluginErr...)
		if b.Tokens > b.Budget {
			warns = append(warns, fmt.Sprintf("brief is %d tokens, over the %d budget (required sections alone exceed it)", b.Tokens, b.Budget))
		}
		return &result{data: b, human: b.Markdown, warnings: warns}, nil
	})
	return c
}

// ---------- tasks ----------

func (a *app) taskCmd() *cobra.Command {
	c := &cobra.Command{Use: "task", Short: "Create, start, list, show, and update tasks"}
	c.AddCommand(a.taskNew(), a.taskStart(), a.taskList(), a.taskShow(), a.taskUpdate(), a.taskBlock(), a.taskClaim(), a.taskRelease())
	return c
}

func (a *app) taskNew() *cobra.Command {
	var in ops.NewTaskInput
	var start bool
	c := &cobra.Command{Use: `new "<title>"`, Short: "Create a task (status todo)", Args: cobra.ExactArgs(1)}
	c.Flags().StringVar(&in.ID, "id", "", "custom task id (default: random T-xxxx)")
	c.Flags().StringArrayVar(&in.Criteria, "ac", nil, `acceptance criterion, e.g. "Given …, when …, then …" (repeatable)`)
	c.Flags().StringArrayVar(&in.Tags, "tag", nil, "tag (repeatable)")
	c.Flags().StringVar(&in.Goal, "goal", "", "goal id (G-…)")
	c.Flags().StringVar(&in.Profile, "profile", "", "pin the risk profile: lite, standard, strict")
	c.Flags().StringVar(&in.Objective, "objective", "", "one-paragraph objective")
	c.Flags().BoolVar(&start, "start", false, "start the task immediately")
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		in.Title = args[0]
		if in.Profile != "" && in.Profile != "lite" && in.Profile != "standard" && in.Profile != "strict" {
			return nil, apperr.Usage("invalid --profile %q", in.Profile)
		}
		t, err := ops.TaskNew(p, in)
		if err != nil {
			return nil, err
		}
		if start {
			if t, err = ops.TaskStart(p, t.ID); err != nil {
				return nil, err
			}
		}
		compat.Write(p)
		h := fmt.Sprintf("created %s %s (%s)\nfile: %s", t.ID, t.Title, t.Status, t.File)
		if !start {
			h += "\nnext: aitk task start " + t.ID
		}
		return &result{data: t, human: h}, nil
	})
	return c
}

func (a *app) taskStart() *cobra.Command {
	c := &cobra.Command{Use: "start <id>", Short: "Make a task active in this worktree (status in_progress)", Args: cobra.ExactArgs(1)}
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		t, err := ops.TaskStart(p, args[0])
		if err != nil {
			return nil, err
		}
		compat.Write(p)
		return &result{data: t, human: fmt.Sprintf("active: %s %s (%s)\nnext: aitk brief", t.ID, t.Title, t.Status)}, nil
	})
	return c
}

func (a *app) taskList() *cobra.Command {
	var status string
	var all bool
	c := &cobra.Command{Use: "list", Short: "List tasks (open ones by default)", Args: cobra.NoArgs}
	c.Flags().StringVar(&status, "status", "", "only this status")
	c.Flags().BoolVar(&all, "all", false, "include done and cancelled")
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		tasks, errs := task.List(p)
		active := session.Load(p).Active()
		var out []*task.Task
		var b strings.Builder
		for _, t := range tasks {
			if status != "" && t.Status != status {
				continue
			}
			if status == "" && !all && (t.Status == task.Done || t.Status == task.Cancelled) {
				continue
			}
			t.Body = ""
			out = append(out, t)
			mark := " "
			if strings.EqualFold(t.ID, active) {
				mark = "*"
			}
			fmt.Fprintf(&b, "%s %-10s %-12s %-8s %s\n", mark, t.ID, t.Status, t.Profile, t.Title)
		}
		if len(out) == 0 {
			b.WriteString("no tasks")
		}
		var warns []string
		for _, e := range errs {
			warns = append(warns, e.Error())
		}
		if out == nil {
			out = []*task.Task{}
		}
		return &result{data: out, human: b.String(), warnings: warns}, nil
	})
	return c
}

func (a *app) taskShow() *cobra.Command {
	c := &cobra.Command{Use: "show <id>", Short: "Print one task", Args: cobra.ExactArgs(1)}
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		t, err := task.Find(p, args[0])
		if err != nil {
			return nil, apperr.TaskNotFound(args[0])
		}
		b, _ := os.ReadFile(p.Path(t.File))
		return &result{data: map[string]any{"task": t, "criteria": t.Criteria()}, human: string(b)}, nil
	})
	return c
}

func (a *app) taskUpdate() *cobra.Command {
	var in ops.UpdateInput
	var acDone string
	c := &cobra.Command{Use: "update [id]", Short: "Edit structured task fields (default: the active task)", Args: cobra.MaximumNArgs(1)}
	c.Flags().StringVar(&in.Status, "status", "", "new status: "+strings.Join(task.Statuses, ", "))
	c.Flags().StringVar(&acDone, "ac-done", "", "mark criteria done, e.g. 1 or 1,3")
	c.Flags().StringArrayVar(&in.AddAC, "add-ac", nil, "add a criterion (repeatable)")
	c.Flags().StringVar(&in.Note, "note", "", "append a timestamped note")
	c.Flags().StringArrayVar(&in.Tags, "tag", nil, "add a tag (repeatable)")
	c.Flags().StringVar(&in.Profile, "profile", "", "pin the risk profile")
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		for _, f := range strings.FieldsFunc(acDone, func(r rune) bool { return r == ',' || r == ' ' }) {
			n, err := strconv.Atoi(f)
			if err != nil {
				return nil, apperr.Usage("--ac-done expects numbers, got %q", f)
			}
			in.ACDone = append(in.ACDone, n)
		}
		id := ""
		if len(args) == 1 {
			id = args[0]
		}
		t, err := ops.TaskUpdate(p, id, in)
		if err != nil {
			return nil, err
		}
		compat.Write(p)
		cs := t.Criteria()
		done := 0
		for _, c := range cs {
			if c.Done {
				done++
			}
		}
		return &result{data: t, human: fmt.Sprintf("updated %s (%s) · criteria %d/%d", t.ID, t.Status, done, len(cs))}, nil
	})
	return c
}

func (a *app) taskBlock() *cobra.Command {
	var reason string
	c := &cobra.Command{Use: "block <id>", Short: "Mark a task blocked with a reason", Args: cobra.ExactArgs(1)}
	c.Flags().StringVar(&reason, "reason", "", "why it is blocked (required)")
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		if strings.TrimSpace(reason) == "" {
			return nil, apperr.Usage("--reason is required")
		}
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		t, err := ops.TaskUpdate(p, args[0], ops.UpdateInput{Status: task.Blocked, Reason: reason})
		if err != nil {
			return nil, err
		}
		compat.Write(p)
		return &result{data: t, human: fmt.Sprintf("%s blocked: %s", t.ID, reason)}, nil
	})
	return c
}

// ---------- verify & finish ----------

func (a *app) checkCmd() *cobra.Command {
	c := &cobra.Command{Use: "check", Short: "Run the project's check command and record evidence on the active task", Args: cobra.NoArgs}
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		r, err := ops.Check(p, !a.json)
		res := &result{data: r}
		if r != nil {
			res.human = fmt.Sprintf("check: exit %d in %d ms", r.ExitCode, r.DurationMs)
			if r.Task == "" {
				res.warnings = append(res.warnings, "no active task: evidence not recorded (aitk task start <id>)")
			}
		}
		return res, err
	})
	return c
}

func (a *app) closeCmd() *cobra.Command {
	var in ops.CloseInput
	c := &cobra.Command{Use: "close", Short: "Finish the session: apply the Definition of Done, write evidence, handoff, and knowledge", Args: cobra.NoArgs}
	c.Flags().StringVar(&in.Summary, "summary", "", "what changed (required)")
	c.Flags().StringVar(&in.Knowledge, "knowledge", "", `lasting finding, or "none" (required)`)
	c.Flags().StringArrayVar(&in.Next, "next", nil, "next step for whoever continues (repeatable)")
	c.Flags().StringVar(&in.Status, "status", "", "hand over unfinished work: in_progress or blocked (skips the Definition of Done)")
	c.Flags().StringArrayVar(&in.Tags, "tag", nil, "tag for the knowledge entry (repeatable)")
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		r, err := ops.Close(p, in)
		if err != nil {
			return nil, err
		}
		compat.Write(p)
		h := fmt.Sprintf("%s → %s (profile %s)\nhandoff: %s", r.Task, r.Status, r.Profile, r.Handoff)
		if r.Knowledge {
			h += "\nknowledge recorded"
		}
		return &result{data: r, human: h, warnings: r.Notes}, nil
	})
	return c
}

func (a *app) reviewCmd() *cobra.Command {
	c := &cobra.Command{Use: "review", Short: "Record bug-hunt review passes (strict tasks)"}
	var findings int
	pass := &cobra.Command{Use: "pass", Short: "Record one review pass on the active task", Args: cobra.NoArgs}
	pass.Flags().IntVar(&findings, "findings", -1, "number of problems found in this pass (required)")
	pass.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		if findings < 0 {
			return nil, apperr.Usage("--findings is required (0 when the pass found nothing)")
		}
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		t, err := ops.ReviewPass(p, findings)
		if err != nil {
			return nil, err
		}
		n := len(t.Review.Passes)
		return &result{data: t.Review, human: fmt.Sprintf("%s review pass %d by %s: %d findings", t.ID, n, t.Review.Passes[n-1].By, findings)}, nil
	})
	c.AddCommand(pass)
	return c
}

// ---------- knowledge & decisions ----------

func (a *app) knowledgeCmd() *cobra.Command {
	c := &cobra.Command{Use: "knowledge", Short: "Add, search, compact, and pin project knowledge"}
	var tags []string
	var pin bool
	add := &cobra.Command{Use: `add "<text>"`, Short: "Add an entry to the inbox", Args: cobra.ExactArgs(1)}
	add.Flags().StringArrayVar(&tags, "tag", nil, "tag (repeatable); the first tag is the topic")
	add.Flags().BoolVar(&pin, "pin", false, "always show in briefs")
	add.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		e := knowledge.Entry{Text: strings.TrimSpace(args[0]), Date: task.Now()[:10], Tags: tagSlugs(tags), Pinned: pin, Source: "manual", Topic: "general"}
		if t := session.Load(p).Active(); t != "" {
			e.Task = t
		}
		if err := schema.Validate("knowledge-entry", e); err != nil {
			return nil, apperr.New("E_SCHEMA", apperr.ExitGate, "keep the entry between 3 and 4000 characters", "%v", err)
		}
		err = ops.WithLock(p, func() error { return knowledge.Add(p, e) })
		if err != nil {
			return nil, err
		}
		return &result{data: e, human: "added to docs/ai/knowledge/_inbox.md"}, nil
	})
	var limit int
	search := &cobra.Command{Use: `search "<query>"`, Short: "Ranked search", Args: cobra.ExactArgs(1)}
	search.Flags().IntVar(&limit, "limit", 10, "max results")
	search.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		hits := knowledge.Search(knowledge.LoadAll(p), args[0], nil, limit)
		var warns []string
		if len(p.Config.Plugins.Enabled) > 0 {
			for _, r := range plugins.Call(p, "knowledge.search", map[string]any{"query": args[0], "limit": limit}, nil) {
				if r.Err != "" {
					warns = append(warns, "plugin "+r.Plugin+": "+r.Err)
					continue
				}
				var ext []struct {
					Text   string  `json:"text"`
					Score  float64 `json:"score"`
					Source string  `json:"source"`
				}
				json.Unmarshal(r.Data, &ext)
				for _, x := range ext {
					if strings.TrimSpace(x.Text) != "" {
						hits = append(hits, knowledge.Scored{Entry: knowledge.Entry{Text: x.Text, Topic: "plugin", Source: "plugin", File: "plugin:" + r.Plugin}, Score: x.Score})
					}
				}
			}
			sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
			if len(hits) > limit {
				hits = hits[:limit]
			}
		}
		var b strings.Builder
		for _, h := range hits {
			fmt.Fprintf(&b, "- %s (%s, %s)\n", textx.Truncate(strings.ReplaceAll(h.Text, "\n", " "), 300), h.Date, h.File)
		}
		if len(hits) == 0 {
			b.WriteString("no matches")
		}
		if hits == nil {
			hits = []knowledge.Scored{}
		}
		return &result{data: hits, human: b.String(), warnings: warns}, nil
	})
	var dry bool
	compact := &cobra.Command{Use: "compact", Short: "File inbox entries by topic, merge duplicates, archive old entries", Args: cobra.NoArgs}
	compact.Flags().BoolVar(&dry, "dry-run", false, "report only")
	compact.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		var r *knowledge.CompactResult
		err = ops.WithLock(p, func() error {
			var err error
			r, err = knowledge.Compact(p, p.Config.ArchiveAfterDays(), dry)
			if err == nil && !dry {
				compat.Write(p)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
		h := fmt.Sprintf("moved %d, merged %d duplicates, archived %d, %d topics", r.Moved, r.Duplicates, r.Archived, len(r.Topics))
		var warns []string
		if len(r.Frozen) > 0 {
			warns = append(warns, "left untouched (contain hand-written prose): "+strings.Join(r.Frozen, ", "))
		}
		return &result{data: r, human: h, warnings: warns}, nil
	})
	pinCmd := &cobra.Command{Use: `pin "<text match>"`, Short: "Pin entries containing the text", Args: cobra.ExactArgs(1)}
	pinCmd.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		var n int
		err = ops.WithLock(p, func() error { var err error; n, err = knowledge.Pin(p, args[0]); return err })
		if err != nil {
			return nil, err
		}
		return &result{data: map[string]int{"pinned": n}, human: fmt.Sprintf("pinned %d entries", n)}, nil
	})
	c.AddCommand(add, search, compact, pinCmd)
	return c
}

func tagSlugs(in []string) []string {
	var out []string
	for _, t := range in {
		out = append(out, project.Slug(t, 62))
	}
	return out
}

func (a *app) adrCmd() *cobra.Command {
	c := &cobra.Command{Use: "adr", Short: "Architecture decision records (MADR 4)"}
	n := &cobra.Command{Use: `new "<title>"`, Short: "Create the next ADR", Args: cobra.ExactArgs(1)}
	n.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		var r *adr.Record
		err = ops.WithLock(p, func() error { var err error; r, err = adr.New(p, args[0]); return err })
		if err != nil {
			return nil, err
		}
		compat.Write(p)
		return &result{data: r, human: "created " + r.File}, nil
	})
	l := &cobra.Command{Use: "list", Short: "List ADRs", Args: cobra.NoArgs}
	l.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		rs := adr.List(p)
		var b strings.Builder
		for _, r := range rs {
			fmt.Fprintf(&b, "ADR-%04d %-10s %s\n", r.Number, r.Status, r.Title)
		}
		if rs == nil {
			rs = []adr.Record{}
			b.WriteString("no ADRs")
		}
		return &result{data: rs, human: b.String()}, nil
	})
	c.AddCommand(n, l)
	return c
}

func (a *app) versionCmd() *cobra.Command {
	c := &cobra.Command{Use: "version", Short: "Print version information", Args: cobra.NoArgs}
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		d := map[string]any{"version": Version, "commit": Commit, "date": Date, "schemas": map[string]int{"state": 2, "session": 1, "result": 1}}
		return &result{data: d, human: fmt.Sprintf("aitk %s (%s, %s)", Version, Commit, Date)}, nil
	})
	return c
}

func (a *app) mcpCmd() *cobra.Command {
	var addr string
	c := &cobra.Command{Use: "mcp", Short: "Serve aitk over the Model Context Protocol (stdio by default)", Args: cobra.NoArgs}
	c.Flags().StringVar(&addr, "http", "", "serve streamable HTTP on this address (e.g. 127.0.0.1:8770) instead of stdio")
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		// Serve even outside an aitk project (e.g. registered globally): each tool then
		// returns E_NOT_A_PROJECT with its fix instead of the server failing to start.
		root := a.dir
		if root == "" {
			root, _ = os.Getwd()
		}
		if p, err := project.Repo(root); err == nil {
			root = p.Root
		}
		exec := func(args []string, stdout, stderr *bytes.Buffer) int { return Execute(args, stdout, stderr) }
		s := mcpserver.New(root, Version, exec)
		var err error
		if addr != "" {
			err = mcpserver.ServeHTTP(addr, s)
		} else {
			err = mcpserver.ServeStdio(cmd.Context(), s)
		}
		if err != nil && cmd.Context().Err() == nil {
			a.fail("mcp", err)
		}
		return nil
	}
	return c
}

func (a *app) adaptersCmd() *cobra.Command {
	c := &cobra.Command{Use: "adapters", Short: "Install aitk into AI tools: instructions, MCP server, session hooks"}
	var tools []string
	var global, dry bool
	sync := &cobra.Command{Use: "sync", Short: "Install or update adapters for detected tools (idempotent)", Args: cobra.NoArgs}
	sync.Flags().StringArrayVar(&tools, "tool", nil, "only this tool (repeatable): "+strings.Join(adapters.Names(), ", "))
	sync.Flags().BoolVar(&global, "global", false, "also edit user-level config (e.g. ~/.codex/config.toml); backed up first")
	sync.Flags().BoolVar(&dry, "dry-run", false, "show what would change")
	sync.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		cs, err := adapters.Plan(adapters.DefaultEnv(p.Root), tools, global)
		if err != nil {
			return nil, apperr.Usage("%v", err)
		}
		var b strings.Builder
		for _, ch := range cs {
			fmt.Fprintf(&b, "%-12s %s (%s)\n", ch.Tool, adapters.Diff(ch), ch.What)
		}
		if len(cs) == 0 {
			b.WriteString("all detected tools are up to date")
		} else if dry {
			b.WriteString("dry run: nothing written")
		} else {
			if err := adapters.Apply(cs, filepath.Join(stateDir(), "backups")); err != nil {
				return nil, err
			}
			b.WriteString("done. Make sure `aitk` is on PATH for every tool; commit the project files so teammates get them.")
		}
		if cs == nil {
			cs = []adapters.Change{}
		}
		return &result{data: map[string]any{"changes": cs, "dry_run": dry}, human: b.String()}, nil
	})
	doc := &cobra.Command{Use: "doctor", Short: "Report which tools are installed and whether their adapters are current", Args: cobra.NoArgs}
	doc.Flags().BoolVar(&global, "global", false, "include user-level config")
	doc.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		st := adapters.Doctor(adapters.DefaultEnv(p.Root), global)
		var b strings.Builder
		var warns []string
		for _, s := range st {
			state := "not installed"
			switch {
			case s.Installed && s.Current:
				state = "ok"
			case s.Installed:
				state = "needs sync: " + strings.Join(s.Pending, "; ")
			}
			fmt.Fprintf(&b, "%-12s %s\n", s.Tool, state)
		}
		if _, err := osexec.LookPath("aitk"); err != nil {
			warns = append(warns, "aitk is not on PATH; tools cannot start the MCP server or hooks")
		}
		return &result{data: st, human: b.String(), warnings: warns}, nil
	})
	c.AddCommand(sync, doc)
	return c
}

// stateDir follows the XDG Base Directory spec.
func stateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "aitk")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "aitk")
}

func (a *app) hooksCmd() *cobra.Command {
	c := &cobra.Command{Use: "hooks", Short: "Install or remove aitk's git hooks (pre-commit, commit-msg, pre-push)"}
	inst := &cobra.Command{Use: "install", Short: "Add aitk to the git hooks (existing hooks are kept)", Args: cobra.NoArgs}
	inst.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		changed, err := gates.Install(p.Root)
		if err != nil {
			return nil, err
		}
		h := "hooks already installed"
		if len(changed) > 0 {
			h = "installed: " + strings.Join(changed, ", ")
		}
		if changed == nil {
			changed = []string{}
		}
		return &result{data: map[string]any{"changed": changed}, human: h}, nil
	})
	un := &cobra.Command{Use: "uninstall", Short: "Remove aitk from the git hooks", Args: cobra.NoArgs}
	un.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := project.Repo(a.cwd())
		if err != nil {
			return nil, err
		}
		changed, err := gates.Uninstall(p.Root)
		if err != nil {
			return nil, err
		}
		if changed == nil {
			changed = []string{}
		}
		return &result{data: map[string]any{"changed": changed}, human: fmt.Sprintf("removed from %d hooks", len(changed))}, nil
	})
	c.AddCommand(inst, un)
	return c
}

func (a *app) cwd() string {
	if a.dir != "" {
		return a.dir
	}
	d, _ := os.Getwd()
	return d
}

func gateErr(vs []gates.Violation, fix string) error {
	var lines []string
	for _, v := range vs {
		lines = append(lines, v.String())
	}
	return apperr.New("E_GATE", apperr.ExitGate, fix, "%d problem(s):\n%s", len(vs), strings.Join(lines, "\n"))
}

// hookCmd is called by the git hooks installed by `aitk hooks install`.
func (a *app) hookCmd() *cobra.Command {
	c := &cobra.Command{Use: "hook <pre-commit|commit-msg|pre-push> [args]", Short: "Run a git hook (called by git)", Hidden: true, Args: cobra.MinimumNArgs(1)}
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		r, err := project.Repo(a.cwd())
		if err != nil {
			return nil, err
		}
		p, perr := project.Open(r.Root)
		switch args[0] {
		case "pre-commit":
			vs, err := gates.PreCommit(r.Root)
			if err != nil {
				return nil, err
			}
			if len(vs) > 0 {
				return &result{data: vs}, gateErr(vs, "fix the problems above and stage again")
			}
			return &result{human: "aitk pre-commit: ok"}, nil
		case "commit-msg":
			if len(args) < 2 {
				return nil, apperr.Usage("commit-msg needs the message file")
			}
			b, err := os.ReadFile(args[1])
			if err != nil {
				return nil, err
			}
			if v := gates.CheckMessage(string(b)); v != nil {
				return &result{data: []gates.Violation{*v}}, gateErr([]gates.Violation{*v}, `git commit -m "feat(scope): summary"`)
			}
			if perr == nil {
				if err := gates.AddTrailers(r.Root, args[1], session.Load(p).Active(), ops.Tool()); err != nil {
					return nil, err
				}
			}
			return &result{}, nil
		case "pre-push":
			if perr != nil || p.Config.Check.Cmd == "" || session.Load(p).Active() == "" {
				return &result{human: "aitk pre-push: nothing to verify"}, nil
			}
			if _, err := ops.Check(p, false); err != nil {
				return nil, apperr.New("E_GATE", apperr.ExitGate, "aitk check (fix the failures), then push again", "pre-push: the check failed for the active task")
			}
			return &result{human: "aitk pre-push: check passed"}, nil
		}
		return nil, apperr.Usage("unknown hook %q", args[0])
	})
	return c
}

func (a *app) ciCmd() *cobra.Command {
	var base string
	c := &cobra.Command{Use: "ci", Short: "Run every gate non-interactively (schemas, secrets, commit messages) for CI", Args: cobra.NoArgs}
	c.Flags().StringVar(&base, "base", "", "compare against this ref (default: PR base from CI env, else merge base with the default branch)")
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		if base == "" {
			base = gates.DefaultBase(p.Root, p.Config.DefaultBranch())
		}
		var report *doctor.Report
		ops.WithLock(p, func() error { report = doctor.Run(p, false); return nil })
		vs, err := gates.CI(p.Root, base)
		if err != nil {
			return nil, err
		}
		for _, ch := range report.Checks {
			if ch.Status == "error" {
				vs = append(vs, gates.Violation{Gate: "doctor", Detail: ch.Name + ": " + ch.Detail})
			}
		}
		data := map[string]any{"base": base, "violations": vs}
		if len(vs) > 0 {
			return &result{data: data}, gateErr(vs, "fix the problems above")
		}
		if vs == nil {
			data["violations"] = []gates.Violation{}
		}
		h := "aitk ci: all gates passed"
		if base == "" {
			h += " (no base to compare; only project files were validated)"
		} else {
			h += " (base " + base + ")"
		}
		return &result{data: data, human: h}, nil
	})
	return c
}

func (a *app) taskClaim() *cobra.Command {
	var ttl time.Duration
	c := &cobra.Command{Use: "claim <id>", Short: "Claim a task for this worktree so other agents skip it", Args: cobra.ExactArgs(1)}
	c.Flags().DurationVar(&ttl, "ttl", ops.DefaultClaimTTL, "how long the claim lasts")
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		cl, err := ops.Claim(p, args[0], ttl)
		if err != nil {
			return nil, err
		}
		return &result{data: cl, human: fmt.Sprintf("claimed %s until %s", cl.Task, cl.Expires)}, nil
	})
	return c
}

func (a *app) taskRelease() *cobra.Command {
	var force bool
	c := &cobra.Command{Use: "release <id>", Short: "Drop this worktree's claim on a task", Args: cobra.ExactArgs(1)}
	c.Flags().BoolVar(&force, "force", false, "drop any worktree's claim (use when that worktree is gone)")
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		ok, err := ops.Release(p, args[0], force)
		if err != nil {
			return nil, err
		}
		h := "no claim to release"
		if ok {
			h = "released " + args[0]
		}
		return &result{data: map[string]bool{"released": ok}, human: h}, nil
	})
	return c
}

func (a *app) workCmd() *cobra.Command {
	c := &cobra.Command{Use: "work <id>", Short: "Create a dedicated worktree and branch for a task, claim it, and start it there", Args: cobra.ExactArgs(1)}
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		r, err := ops.Work(p, args[0])
		if err != nil {
			return nil, err
		}
		verb := "reusing"
		if r.Created {
			verb = "created"
		}
		h := fmt.Sprintf("%s worktree %s on branch %s; %s is active there\nnext: cd %q && aitk brief", verb, r.Path, r.Branch, r.Task, r.Path)
		return &result{data: r, human: h}, nil
	})
	return c
}

func (a *app) switchCmd() *cobra.Command {
	var note string
	var printOnly bool
	c := &cobra.Command{Use: "switch <tool>", Short: "Hand the work to another AI tool: write a handoff and start it with the brief", Args: cobra.ExactArgs(1)}
	c.Flags().StringVar(&note, "note", "", "why you are switching / what the next tool should know")
	c.Flags().BoolVar(&printOnly, "print", false, "print how to start the tool instead of starting it")
	c.RunE = func(cmd *cobra.Command, args []string) error {
		p, err := a.open()
		if err != nil {
			a.fail("switch", err)
			return nil
		}
		r, err := ops.Switch(p, args[0], note)
		if err != nil {
			a.fail("switch", err)
			return nil
		}
		interactive := !a.json && !printOnly && isTTY(os.Stdout) && len(r.Command) > 0
		if interactive {
			if _, err := osexec.LookPath(r.Command[0]); err == nil {
				fmt.Fprintf(a.stderr, "handoff written: %s\nstarting %s…\n", r.Handoff, r.Command[0])
				c := osexec.Command(r.Command[0], r.Command[1:]...)
				c.Dir, c.Stdin, c.Stdout, c.Stderr = p.Root, os.Stdin, os.Stdout, os.Stderr
				if err := c.Run(); err != nil {
					a.code = apperr.ExitRuntime
				}
				return nil
			}
		}
		h := "handoff written: " + r.Handoff + "\nprompt: " + r.PromptFile + "\n"
		if len(r.Command) > 0 {
			h += fmt.Sprintf("start: %s \"$(cat %s)\"", strings.Join(r.Command[:len(r.Command)-1], " "), r.PromptFile)
		} else {
			h += "start " + r.Tool + " in this repository and paste the prompt file as the first message (known start commands: " + strings.Join(ops.SwitchTools(), ", ") + ")"
		}
		data := *r
		data.Prompt = "" // in prompt_file; keeps JSON output small
		a.ok("switch", &result{data: data, human: h})
		return nil
	}
	return c
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func (a *app) reportCmd() *cobra.Command {
	var since string
	var format string
	c := &cobra.Command{Use: "report", Short: "What was done, what is in progress or blocked, and which tools did it", Args: cobra.NoArgs}
	c.Flags().StringVar(&since, "since", "7d", "period: e.g. 24h, 7d, 30d, or a date (2026-10-01)")
	c.Flags().StringVar(&format, "format", "md", "md or json (json is also --json)")
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		from, err := parseSince(since)
		if err != nil {
			return nil, apperr.Usage("%v", err)
		}
		r := ops.BuildReport(p, from)
		day := from.UTC().Format("2006-01-02")
		for _, e := range knowledge.LoadAll(p) {
			if e.Date >= day {
				r.Knowledge++
			}
		}
		h := r.Markdown(p.Config.Project.Name)
		if format == "json" {
			b, _ := json.MarshalIndent(r, "", "  ")
			h = string(b)
		}
		return &result{data: r, human: h}, nil
	})
	return c
}

func parseSince(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err == nil && n >= 0 {
			return time.Now().AddDate(0, 0, -n), nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since %q (use 24h, 7d, or 2026-10-01)", s)
	}
	return time.Now().Add(-d), nil
}

func (a *app) logCmd() *cobra.Command {
	var id string
	var limit int
	c := &cobra.Command{Use: "log", Short: "Handoffs, newest first: who (which tool) did what", Args: cobra.NoArgs}
	c.Flags().StringVar(&id, "task", "", "only this task")
	c.Flags().IntVar(&limit, "limit", 20, "max entries")
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		var out []*handoff.Handoff
		var b strings.Builder
		for _, h := range handoff.List(p) {
			if id != "" && !strings.EqualFold(h.Task, id) {
				continue
			}
			if len(out) == limit {
				break
			}
			out = append(out, h)
			task := h.Task
			if task == "" {
				task = "-"
			}
			fmt.Fprintf(&b, "%s  %-8s %-12s %-9s %s\n", h.At, task, h.By, h.Outcome, textx.Truncate(textx.FirstLine(strings.TrimPrefix(strings.TrimSpace(h.Body), "## ")), 80))
		}
		if out == nil {
			out = []*handoff.Handoff{}
			b.WriteString("no handoffs")
		}
		return &result{data: out, human: b.String()}, nil
	})
	return c
}

func (a *app) pluginCmd() *cobra.Command {
	c := &cobra.Command{Use: "plugin", Short: "Plugins: aitk-<name> executables on PATH, enabled in aitk.toml [plugins] enabled"}
	l := &cobra.Command{Use: "list", Short: "List discovered plugins, their hooks, and whether they are enabled", Args: cobra.NoArgs}
	l.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		var cfg project.Config
		if p, err := a.open(); err == nil {
			cfg = p.Config
		}
		ps := plugins.Discover(cfg)
		var b strings.Builder
		for i := range ps {
			if err := plugins.Manifest(cfg, &ps[i]); err != nil {
				ps[i].Error = err.Error()
			}
			state := "disabled"
			if ps[i].Enabled {
				state = "enabled"
			}
			detail := strings.Join(ps[i].Hooks, ", ")
			if ps[i].Error != "" {
				detail = "error: " + ps[i].Error
			}
			fmt.Fprintf(&b, "%-20s %-8s %-9s %s\n", ps[i].Name, ps[i].Version, state, detail)
		}
		if len(ps) == 0 {
			b.WriteString("no plugins found (executables named aitk-<name> on PATH)")
			ps = []plugins.Plugin{}
		}
		return &result{data: ps, human: b.String()}, nil
	})
	c.AddCommand(l)
	return c
}

func (a *app) importCmd() *cobra.Command {
	c := &cobra.Command{Use: "import", Short: "Import tasks from GitHub Issues, Spec Kit, or OpenSpec"}
	var repo, label, state string
	var limit int
	var dry bool
	gh := &cobra.Command{Use: "github-issues", Short: "Import issues as tasks (GH-<number>) via the gh CLI", Args: cobra.NoArgs}
	gh.Flags().StringVar(&repo, "repo", "", "owner/name (default: this repository)")
	gh.Flags().StringVar(&label, "label", "", "only issues with this label")
	gh.Flags().StringVar(&state, "state", "open", "open, closed, or all")
	gh.Flags().IntVar(&limit, "limit", 100, "max issues")
	gh.Flags().BoolVar(&dry, "dry-run", false, "show what would be imported")
	gh.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		r, err := ops.ImportGitHub(p, repo, label, state, limit, dry)
		if err != nil {
			return nil, err
		}
		if !dry {
			compat.Write(p)
		}
		return &result{data: r, human: importSummary(r)}, nil
	})
	c.AddCommand(gh)
	for _, kind := range []string{"spec-kit", "openspec"} {
		kind := kind
		var dry bool
		k := &cobra.Command{Use: kind + " [tasks.md ...]", Short: "Import a " + kind + " tasks checklist (default: discovered tasks.md files)"}
		k.Flags().BoolVar(&dry, "dry-run", false, "show what would be imported")
		k.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
			p, err := a.open()
			if err != nil {
				return nil, err
			}
			r, err := ops.ImportChecklist(p, kind, args, dry)
			if err != nil {
				return nil, err
			}
			if !dry {
				compat.Write(p)
			}
			return &result{data: r, human: importSummary(r)}, nil
		})
		c.AddCommand(k)
	}
	return c
}

func importSummary(r *ops.ImportResult) string {
	verb := "imported"
	if r.DryRun {
		verb = "would import"
	}
	h := fmt.Sprintf("%s %d task(s) from %s", verb, len(r.Imported), r.Source)
	if len(r.Skipped) > 0 {
		h += fmt.Sprintf("; %d already present", len(r.Skipped))
	}
	return h
}

func (a *app) deployCmd() *cobra.Command {
	c := &cobra.Command{Use: "deploy [target]", Short: "Run a deploy target from aitk.toml, then deploy.post_check", Args: cobra.MaximumNArgs(1)}
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		targets := p.Config.Deploy.Targets
		if len(args) == 0 {
			var names []string
			for k := range targets {
				names = append(names, k)
			}
			sort.Strings(names)
			if len(names) == 0 {
				return nil, apperr.New("E_USAGE", apperr.ExitUsage, "add [deploy.targets] production = \"<command>\" to aitk.toml", "no deploy targets configured")
			}
			return &result{data: names, human: "targets: " + strings.Join(names, ", ")}, nil
		}
		cmdline, ok := targets[args[0]]
		if !ok {
			return nil, apperr.Usage("unknown deploy target %q", args[0])
		}
		run := func(cmd string) checkrun.Result { return checkrun.Run(p.Root, cmd, 2*time.Hour, !a.json) }
		r := run(cmdline)
		data := map[string]any{"target": args[0], "exit_code": r.ExitCode, "duration_ms": r.DurationMs}
		if r.ExitCode != 0 {
			return &result{data: data}, apperr.New("E_DEPLOY_FAILED", apperr.ExitRuntime, "fix the failure and run aitk deploy "+args[0]+" again", "deploy %s failed (exit %d)", args[0], r.ExitCode)
		}
		if pc := p.Config.Deploy.PostCheck; pc != "" {
			pr := run(pc)
			data["post_check_exit_code"] = pr.ExitCode
			if pr.ExitCode != 0 {
				return &result{data: data}, apperr.New("E_DEPLOY_FAILED", apperr.ExitRuntime, "investigate the post-deploy check; consider rolling back", "post-deploy check failed (exit %d)", pr.ExitCode)
			}
		}
		return &result{data: data, human: fmt.Sprintf("deployed %s in %d ms", args[0], r.DurationMs)}, nil
	})
	return c
}
