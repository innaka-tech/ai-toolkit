// Package doc handles Markdown files with YAML frontmatter and level-2 sections.
package doc

import (
	"bytes"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Split separates YAML frontmatter from the body. ok is false when there is no frontmatter.
func Split(content string) (front, body string, ok bool) {
	c := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(c, "---\n") {
		return "", c, false
	}
	rest := c[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			return rest[:len(rest)-4], "", true
		}
		return "", c, false
	}
	return rest[:end], strings.TrimLeft(rest[end+5:], "\n"), true
}

// Join renders frontmatter (from v) and body.
func Join(v any, body string) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	enc.Close()
	out := "---\n" + buf.String() + "---\n\n" + strings.TrimRight(body, "\n") + "\n"
	return []byte(out), nil
}

// Section returns the body of "## <name>" (case-insensitive), without the heading.
func Section(body, name string) (string, bool) {
	start, end := find(body, name)
	if start < 0 {
		return "", false
	}
	return strings.Trim(body[start:end], "\n"), true
}

// SetSection replaces (or appends) the "## <name>" section body.
func SetSection(body, name, content string) string {
	start, end := find(body, name)
	block := strings.TrimRight(content, "\n") + "\n"
	if start < 0 {
		return strings.TrimRight(body, "\n") + "\n\n## " + name + "\n" + block
	}
	tail := body[end:]
	if tail != "" {
		block += "\n"
	}
	return body[:start] + block + tail
}

// find returns the byte range of the section body (after the heading line).
func find(body, name string) (int, int) {
	lines := strings.SplitAfter(body, "\n")
	pos, start := 0, -1
	inFence := false
	for _, l := range lines {
		t := strings.TrimRight(l, "\n")
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(t, "## ") {
			if start >= 0 {
				return start, pos
			}
			if strings.EqualFold(strings.TrimSpace(t[3:]), name) {
				start = pos + len(l)
			}
		}
		pos += len(l)
	}
	if start >= 0 {
		return start, len(body)
	}
	return -1, -1
}
