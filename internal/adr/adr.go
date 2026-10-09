// Package adr creates and lists MADR 4 decision records in docs/adr/.
package adr

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/internal/doc"
	"github.com/innaka-tech/ai-toolkit/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/internal/project"
	"github.com/innaka-tech/ai-toolkit/internal/textx"
)

// Record is one ADR.
type Record struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Date   string `json:"date"`
	File   string `json:"file"`
}

var nameRe = regexp.MustCompile(`^(\d{4})-.+\.md$`)
var kvRe = regexp.MustCompile(`(?m)^(status|date):\s*(.+)$`)

// List returns ADRs ordered by number.
func List(p *project.Project) []Record {
	entries, _ := os.ReadDir(p.Path(project.ADRDir))
	var out []Record
	for _, e := range entries {
		m := nameRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		rel := filepath.ToSlash(filepath.Join(project.ADRDir, e.Name()))
		b, _ := os.ReadFile(p.Path(rel))
		r := Record{Number: n, File: rel}
		front, body, _ := doc.Split(string(b))
		for _, kv := range kvRe.FindAllStringSubmatch(front, -1) {
			if kv[1] == "status" {
				r.Status = strings.TrimSpace(kv[2])
			} else {
				r.Date = strings.Trim(strings.TrimSpace(kv[2]), `"'`)
			}
		}
		r.Title = strings.TrimSpace(regexp.MustCompile(`^ADR-\d+:\s*`).ReplaceAllString(textx.FirstLine(body), ""))
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// New writes the next-numbered ADR from the MADR template.
func New(p *project.Project, title string) (*Record, error) {
	next := 1
	if rs := List(p); len(rs) > 0 {
		next = rs[len(rs)-1].Number + 1
	}
	date := time.Now().UTC().Format("2006-01-02")
	rel := filepath.ToSlash(filepath.Join(project.ADRDir, fmt.Sprintf("%04d-%s.md", next, project.Slug(title, 60))))
	body := fmt.Sprintf(`---
status: proposed
date: %s
---

# ADR-%04d: %s

## Context and Problem Statement

## Considered Options

## Decision Outcome

Chosen option: "", because

### Consequences

* Good, because
* Bad, because
`, date, next, title)
	if err := fsx.WriteFile(p.Path(rel), []byte(body), 0o644); err != nil {
		return nil, err
	}
	return &Record{Number: next, Title: title, Status: "proposed", Date: date, File: rel}, nil
}
