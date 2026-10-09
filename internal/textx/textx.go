// Package textx has small UTF-8-safe text helpers.
package textx

import "strings"

// Truncate shortens s to at most n runes, appending "…" when cut.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// FirstLine returns the first non-empty line, stripped of Markdown heading marks.
func FirstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "#"))
		if l != "" {
			return l
		}
	}
	return ""
}

// Tokens estimates tokens as ceil(utf8 bytes / 4) (docs/spec/workflow.md §7).
func Tokens(s string) int { return (len(s) + 3) / 4 }
