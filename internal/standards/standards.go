// Package standards serves aitk's development standards: the built-in set embedded in the
// binary, overridden or extended by a project's own files in docs/ai/standards/.
package standards

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	builtin "github.com/innaka-tech/ai-toolkit/v2/standards"
)

// ProjectDir holds a project's own standards, one markdown file per topic.
const ProjectDir = "docs/ai/standards"

// Standard is one topic. Title is the first "# " heading; Summary the first paragraph under it.
type Standard struct {
	Topic   string `json:"topic"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Body    string `json:"-"`
	Source  string `json:"source"` // "builtin" or the project file
}

var topicRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)

// maxStandard bounds a file so a standard stays readable in one agent turn.
const maxStandard = 16000

func parse(topic, source string, b []byte) (Standard, error) {
	if !topicRE.MatchString(topic) {
		return Standard{}, fmt.Errorf("%s: topic names use lowercase letters, digits, and '-'", source)
	}
	if len(b) > maxStandard {
		return Standard{}, fmt.Errorf("%s: larger than %d KB; split the topic", source, maxStandard/1000)
	}
	s := Standard{Topic: topic, Source: source, Body: strings.ReplaceAll(string(b), "\r\n", "\n")}
	inPara := false
	for _, l := range strings.Split(s.Body, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case s.Title == "" && strings.HasPrefix(t, "# "):
			s.Title = strings.TrimPrefix(t, "# ")
		case s.Title != "" && t == "" && inPara:
			return s, nil
		case s.Title != "" && t != "" && !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "<!--"):
			inPara = true
			s.Summary = strings.TrimSpace(s.Summary + " " + strings.TrimPrefix(t, "> "))
		}
	}
	return s, nil
}

// All returns the built-in standards with the project's own merged in (a project file with a
// built-in's topic replaces it), sorted by topic, plus warnings for files that were skipped.
func All(root string) ([]Standard, []string) {
	by := map[string]Standard{}
	var warns []string
	ents, _ := fs.ReadDir(builtin.FS, ".")
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, _ := fs.ReadFile(builtin.FS, e.Name())
		if s, err := parse(strings.TrimSuffix(e.Name(), ".md"), "builtin", b); err == nil {
			by[s.Topic] = s
		}
	}
	files, _ := filepath.Glob(filepath.Join(root, ProjectDir, "*.md"))
	for _, f := range files {
		rel := filepath.ToSlash(filepath.Join(ProjectDir, filepath.Base(f)))
		b, err := os.ReadFile(f)
		if err != nil {
			warns = append(warns, rel+": "+err.Error())
			continue
		}
		s, err := parse(strings.TrimSuffix(filepath.Base(f), ".md"), rel, b)
		if err != nil {
			warns = append(warns, err.Error())
			continue
		}
		by[s.Topic] = s
	}
	out := make([]Standard, 0, len(by))
	for _, s := range by {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Topic < out[j].Topic })
	return out, warns
}

// Find returns one standard by topic.
func Find(root, topic string) (Standard, bool) {
	all, _ := All(root)
	for _, s := range all {
		if s.Topic == topic {
			return s, true
		}
	}
	return Standard{}, false
}
