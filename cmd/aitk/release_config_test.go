package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The release workflow passes CHANGELOG.md's section to GoReleaser with --release-notes.
// GoReleaser skips its whole changelog pipe, including that file, when changelog.disable is
// true (or a template rendering to "true"), which published every v2 release before this fix
// with empty release notes.
func TestReleaseNotesReachGoReleaser(t *testing.T) {
	wf, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wf), "--release-notes") {
		t.Skip("release.yml no longer passes --release-notes")
	}
	b, err := os.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Changelog struct {
			Disable any `yaml:"disable"`
		} `yaml:"changelog"`
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	// GoReleaser renders the value as a template and compares it with "true", case-insensitively.
	d := strings.ToLower(strings.TrimSpace(fmt.Sprint(cfg.Changelog.Disable)))
	if d == "true" || strings.Contains(d, "{{") {
		t.Fatalf(".goreleaser.yaml sets changelog.disable to %q, so GoReleaser can ignore --release-notes and publish an empty release body", d)
	}
}
