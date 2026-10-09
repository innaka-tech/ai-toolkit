package agentsmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docs/spec/workflow.md §2: the block stays ≤ 25 lines and uses shell commands only.
func TestBlockFollowsTheSpec(t *testing.T) {
	if n := strings.Count(Block, "\n") + 1; n > 25 {
		t.Fatalf("block has %d lines, the spec allows 25", n)
	}
	if strings.Contains(Block, "mcp__") {
		t.Fatal("the block must not depend on MCP")
	}
	spec, err := os.ReadFile(filepath.Join("..", "..", "docs", "spec", "workflow.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ReplaceAll(string(spec), "\r\n", "\n"), Block) {
		t.Fatal("docs/spec/workflow.md §2 shows a different block; update the spec")
	}
}

func TestApplyKeepsUserTextAndReplacesOldBlocks(t *testing.T) {
	user := "# Project\n\nOur rules.\n"
	got := Apply(user)
	if !strings.HasPrefix(got, user) || Inspect(got) != Current {
		t.Fatalf("block not appended:\n%s", got)
	}
	if Apply(got) != got {
		t.Fatal("Apply is not idempotent")
	}
	old := user + "\n" + Begin + "\nold steps\n" + End + "\ntail\n"
	if got := Apply(old); !strings.Contains(got, "tail") || strings.Contains(got, "old steps") || Inspect(got) != Current {
		t.Fatalf("outdated block not replaced in place:\n%s", got)
	}
	v1 := user + v1Begin + "\nv1 protocol\n" + v1End + "\n"
	if got := Apply(v1); strings.Contains(got, "v1 protocol") || !strings.Contains(got, "Our rules.") {
		t.Fatalf("v1 block not replaced:\n%s", got)
	}
}
