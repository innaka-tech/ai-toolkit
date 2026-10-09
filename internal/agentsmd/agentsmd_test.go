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
	if Inspect(old) != Edited || Inspect(user+previous[0]+"\n") != Outdated {
		t.Fatal("hand-edited and older-release blocks must be told apart")
	}
	if Inspect(strings.ReplaceAll(got, "\n", "\r\n")) != Current {
		t.Fatal("a CRLF checkout of the current block is current")
	}
	if got := Apply(old); !strings.Contains(got, "tail") || strings.Contains(got, "old steps") || Inspect(got) != Current {
		t.Fatalf("outdated block not replaced in place:\n%s", got)
	}
	v1 := user + v1Begin + "\nv1 protocol\n" + v1End + "\n"
	if got := Apply(v1); strings.Contains(got, "v1 protocol") || !strings.Contains(got, "Our rules.") {
		t.Fatalf("v1 block not replaced:\n%s", got)
	}
}

func TestNewerReleaseBlockIsKept(t *testing.T) {
	newer := strings.Replace(Block, "block-revision 2", "block-revision 9", 1)
	newer = strings.Replace(newer, "Run `aitk brief`", "Run `aitk brief --full`", 1)
	content := "# P\n\n" + newer + "\n"
	if Inspect(content) != Newer {
		t.Fatalf("a higher revision is a newer release, got %v", Inspect(content))
	}
	path := t.TempDir() + "/AGENTS.md"
	os.WriteFile(path, []byte(content), 0o644)
	if changed, _ := Sync(path); changed {
		t.Fatal("an older aitk must not replace a newer block")
	}
}
