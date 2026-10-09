package cli

import (
	"strings"
	"testing"
)

func TestConventionsReachEveryBrief(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	md := data(aitk(t, dir, "brief"))["markdown"].(string)
	if !strings.Contains(md, "Not written yet") {
		t.Fatalf("template conventions should prompt to fill them:\n%s", md)
	}
	if !strings.Contains(aitk(t, dir, "doctor").stdout, "still the template") {
		t.Fatal("doctor should flag unfilled conventions")
	}
	write(t, dir, "docs/ai/conventions.md", "# Conventions\n\n- Stack: Laravel 11, Nuxt 3, PostgreSQL 16\n- Money: decimal strings, never floats\n- Times: stored UTC, shown WIB\n")
	md = data(aitk(t, dir, "brief"))["markdown"].(string)
	for _, want := range []string{"## Conventions", "Money: decimal strings, never floats", "Times: stored UTC, shown WIB"} {
		if !strings.Contains(md, want) {
			t.Fatalf("brief lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(aitk(t, dir, "doctor").stdout, "still the template") {
		t.Fatal("filled conventions still flagged")
	}
}

func TestGoalsTrackProgress(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	expect(t, aitk(t, dir, "task", "new", "Cart", "--goal", "G-9"), 2, "E_USAGE")
	g := aitk(t, dir, "goal", "add", "Customers can check out")
	mustOK(t, g)
	if data(g)["id"] != "G-1" {
		t.Fatalf("goal id: %v", data(g))
	}
	mustOK(t, aitk(t, dir, "goal", "status", "G-1", "active"))
	newStarted(t, dir, "Cart totals", "--goal", "G-1", "--ac", "totals shown")
	if md := data(aitk(t, dir, "brief"))["markdown"].(string); !strings.Contains(md, "Goal: G-1 Customers can check out [active] (0/1 tasks done)") {
		t.Fatalf("brief lacks the goal:\n%s", md)
	}
	write(t, dir, "ok.txt", "1")
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "task", "update", "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "close", "--summary", "Totals", "--knowledge", "none"))
	gl := aitk(t, dir, "goal", "list")
	if goals := gl.env["data"].([]any); goals[0].(map[string]any)["done"].(float64) != 1 {
		t.Fatalf("goal progress: %v", goals)
	}
	if !strings.Contains(read(t, dir, "docs/ai/goals.md"), "G-1 [active] Customers can check out — 1/1 tasks done") {
		t.Fatalf("goals.md:\n%s", read(t, dir, "docs/ai/goals.md"))
	}
	expect(t, aitk(t, dir, "goal", "status", "G-1", "finished"), 2, "E_USAGE")
}
