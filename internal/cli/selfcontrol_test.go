package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFailureStreakTellsAgentsToStop(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	newStarted(t, dir, "Flaky fix", "--ac", "passes")
	first := aitk(t, dir, "check")
	expect(t, first, 1, "E_CHECK_FAILED")
	if fix := first.env["error"].(map[string]any)["fix"].(string); strings.Contains(fix, "stop") {
		t.Fatal("first failure should not yet say stop")
	}
	second := aitk(t, dir, "check")
	e := second.env["error"].(map[string]any)
	if !strings.Contains(e["message"].(string), "failure 2 in a row") || !strings.Contains(e["fix"].(string), "stop and ask the user") {
		t.Fatalf("second failure must tell the agent to stop: %v", e)
	}
	if md := data(aitk(t, dir, "brief"))["markdown"].(string); !strings.Contains(md, "failed 2 times in a row") {
		t.Fatalf("brief does not warn:\n%s", md)
	}
	write(t, dir, "ok.txt", "1")
	ok := aitk(t, dir, "check")
	mustOK(t, ok)
	if md := data(aitk(t, dir, "brief"))["markdown"].(string); strings.Contains(md, "in a row") {
		t.Fatal("a passing check must reset the streak")
	}
}

func TestSecretsAreNeverRecorded(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	token := "gh" + "p_" + "aB3dE5fG7hI9jK1lM3nO5pQ7rS9tU1vW3xY5" // built at runtime
	expect(t, aitk(t, dir, "knowledge", "add", "Deploy uses "+token), 3, "E_SECRET")
	expect(t, aitk(t, dir, "task", "new", "Rotate keys", "--ac", "old key "+token+" revoked"), 3, "E_SECRET")
	newStarted(t, dir, "Rotate keys", "--ac", "old key revoked")
	expect(t, aitk(t, dir, "task", "update", "--note", "tried "+token), 3, "E_SECRET")
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	expect(t, aitk(t, dir, "close", "--summary", "Rotated "+token, "--knowledge", "none"), 3, "E_SECRET")
	expect(t, aitk(t, dir, "close", "--summary", "Rotated", "--knowledge", "new token is "+token), 3, "E_SECRET")
	expect(t, aitk(t, dir, "switch", "codex", "--note", token, "--print"), 3, "E_SECRET")
	filepath.Walk(filepath.Join(dir, "docs"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), token) {
				t.Fatalf("secret written to %s", p)
			}
		}
		return nil
	})
	mustOK(t, aitk(t, dir, "close", "--summary", "Rotated the deploy key", "--knowledge", "Deploy key lives in DEPLOY_TOKEN"))
}

func TestLargeChangeIsFlagged(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-qm", "aitk init")
	newStarted(t, dir, "Big refactor", "--ac", "still works")
	for i := 0; i < 41; i++ {
		write(t, dir, fmt.Sprintf("src/f%02d.txt", i), "x\n")
	}
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	r := aitk(t, dir, "close", "--summary", "Refactored", "--knowledge", "none")
	mustOK(t, r)
	if ws := r.env["warnings"].([]any); len(ws) == 0 || !strings.Contains(fmt.Sprint(ws), "large change") {
		t.Fatalf("expected a large-change warning, got %v", ws)
	}
}
