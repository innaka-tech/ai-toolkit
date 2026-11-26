package main

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The release workflow passes CHANGELOG.md's section to GoReleaser with --release-notes.
// GoReleaser skips its whole changelog pipe, including that file, when changelog.disable is
// true, which published v2.6.0 and v2.7.0 with empty release notes.
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
	if d := cfg.Changelog.Disable; d == true || d == "true" {
		t.Fatal(".goreleaser.yaml sets changelog.disable: true, so GoReleaser ignores --release-notes and publishes an empty release body")
	}
}
