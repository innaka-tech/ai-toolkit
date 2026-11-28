package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/innaka-tech/ai-toolkit/v2/internal/apperr"
	"github.com/innaka-tech/ai-toolkit/v2/internal/ops"
	"github.com/innaka-tech/ai-toolkit/v2/internal/slop"
	"github.com/innaka-tech/ai-toolkit/v2/internal/standards"
)

func (a *app) standardsCmd() *cobra.Command {
	c := &cobra.Command{Use: "standards [topic]", Short: "List the development standards, or print one topic (built-in, overridden by docs/ai/standards/*.md)",
		Long: "aitk ships development standards for every project: character (how agents work), sdlc, engineering, testing, security,\n" +
			"api, database, operations, git, review, documentation, ai-agents, ux, anti-slop. A project overrides a topic, or adds its own,\n" +
			"with docs/ai/standards/<topic>.md. Personas (aitk prompt) audit and correct code against these topics, e.g.\n" +
			"  aitk standards security",
		Args: cobra.MaximumNArgs(1)}
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		root := "."
		if p, err := a.open(); err == nil {
			root = p.Root
		}
		if len(args) == 0 {
			all, warns := standards.All(root)
			var b strings.Builder
			for _, s := range all {
				src := ""
				if s.Source != "builtin" {
					src = "  (" + s.Source + ")"
				}
				fmt.Fprintf(&b, "%-14s %s%s\n", s.Topic, s.Title, src)
			}
			b.WriteString("\nnext: aitk standards <topic>")
			return &result{data: all, human: b.String(), warnings: warns}, nil
		}
		s, ok := standards.Find(root, args[0])
		if !ok {
			return nil, apperr.New("E_USAGE", apperr.ExitUsage, "aitk standards (lists the topics)", "no standard %q", args[0])
		}
		return &result{data: map[string]any{"topic": s.Topic, "title": s.Title, "source": s.Source, "body": s.Body}, human: strings.TrimRight(s.Body, "\n")}, nil
	})
	return c
}

func (a *app) debtCmd() *cobra.Command {
	c := &cobra.Command{Use: "debt", Short: "List deliberate shortcuts recorded in code as 'shortcut: <the limit>, <when to upgrade>' comments", Args: cobra.NoArgs}
	c.RunE = a.wrap(func(*cobra.Command, []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		items := ops.Debt(p)
		var b strings.Builder
		for _, s := range items {
			fmt.Fprintf(&b, "%s:%d  %s\n", s.File, s.Line, s.Text)
		}
		if len(items) == 0 {
			b.WriteString("no shortcuts recorded\n")
		} else {
			fmt.Fprintf(&b, "%d shortcut(s); turn the ones that now matter into tasks: aitk task new \"<upgrade>\" --ac \"…\"\n", len(items))
		}
		if items == nil {
			items = []ops.Shortcut{}
		}
		return &result{data: items, human: strings.TrimRight(b.String(), "\n")}, nil
	})
	return c
}

func (a *app) slopCmd() *cobra.Command {
	var all bool
	c := &cobra.Command{Use: "slop [path…]", Short: "Find AI-slop in the change: filler phrases, emoji headings, comments that restate code, swallowed errors, placeholders",
		Long: "Scans the files changed on this branch (or the given paths, or every tracked file with --all) for patterns that\n" +
			"are almost always generic AI output. It advises and never fails a command: fix the hits or mark a deliberate one\n" +
			"with 'aitk-slop: ignore' on the line ('aitk-slop: ignore-file' for a whole file). See aitk standards anti-slop."}
	c.Flags().BoolVar(&all, "all", false, "scan every tracked file, not only the change")
	c.RunE = a.wrap(func(_ *cobra.Command, args []string) (*result, error) {
		p, err := a.open()
		if err != nil {
			return nil, err
		}
		paths := args
		if all {
			paths = ops.TrackedFiles(p)
		}
		hits := ops.Slop(p, paths)
		var b strings.Builder
		for _, h := range hits {
			fmt.Fprintf(&b, "%s:%d  [%s] %s\n    %s\n", h.File, h.Line, h.Rule, h.Excerpt, h.Why)
		}
		if len(hits) == 0 {
			b.WriteString("no AI-slop found\n")
		} else {
			fmt.Fprintf(&b, "%d hit(s); fix them or run /aitk-deslop fix\n", len(hits))
		}
		if hits == nil {
			hits = []slop.Hit{}
		}
		return &result{data: hits, human: strings.TrimRight(b.String(), "\n")}, nil
	})
	return c
}
