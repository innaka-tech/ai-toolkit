package cli

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innaka-tech/ai-toolkit/internal/knowledge"
)

// TestRandomOperationsNeverCorruptState runs 1000 random commands against one project and
// checks the release-gate invariants: no panic, exit codes from the spec, valid JSON
// envelopes (checked by aitk()), zero doctor errors, and no knowledge text ever lost.
func TestRandomOperationsNeverCorruptState(t *testing.T) {
	if testing.Short() {
		t.Skip("long-running property test")
	}
	steps := 1000
	if s := os.Getenv("AITK_PROPERTY_STEPS"); s != "" {
		fmt.Sscan(s, &steps)
	}
	dir := repo(t)
	initRepo(t, dir)
	rng := rand.New(rand.NewSource(20261009))
	words := []string{"invoice", "cart", "auth", "laporan", "laba", "💰", "ünïcödé", "deploy", "cron", "margin", "refund", "UI", "API", "db"}
	phrase := func(n int) string {
		var p []string
		for i := 0; i < n; i++ {
			p = append(p, words[rng.Intn(len(words))])
		}
		return strings.Join(p, " ")
	}
	var ids []string
	var added []string // every knowledge text successfully recorded
	counts := map[string]int{}
	validExit := map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true}

	pick := func() string {
		if len(ids) == 0 {
			return "T-none"
		}
		return ids[rng.Intn(len(ids))]
	}
	ops := []struct {
		name string
		args func() []string
	}{
		{"task-new", func() []string {
			a := []string{"task", "new", "Do " + phrase(3)}
			for i := rng.Intn(3); i > 0; i-- {
				a = append(a, "--ac", "Given "+phrase(2)+", then "+phrase(2))
			}
			if rng.Intn(4) == 0 {
				a = append(a, "--start")
			}
			return a
		}},
		{"task-start", func() []string { return []string{"task", "start", pick()} }},
		{"ac-done", func() []string { return []string{"task", "update", "--ac-done", fmt.Sprint(1 + rng.Intn(3))} }},
		{"add-ac", func() []string { return []string{"task", "update", "--add-ac", phrase(4)} }},
		{"note", func() []string { return []string{"task", "update", "--note", phrase(5)} }},
		{"status", func() []string {
			return []string{"task", "update", pick(), "--status", []string{"todo", "in_progress", "blocked", "cancelled"}[rng.Intn(4)]}
		}},
		{"knowledge", func() []string {
			text := phrase(6)
			if rng.Intn(5) == 0 {
				text += "\n" + phrase(4) // multi-line
			}
			if rng.Intn(6) == 0 && len(added) > 0 {
				text = added[rng.Intn(len(added))] // duplicate
			}
			a := []string{"knowledge", "add", text}
			if rng.Intn(2) == 0 {
				a = append(a, "--tag", words[rng.Intn(4)])
			}
			return a
		}},
		{"compact", func() []string { return []string{"knowledge", "compact"} }},
		{"pin", func() []string { return []string{"knowledge", "pin", words[rng.Intn(len(words))]} }},
		{"toggle-ok", nil},
		{"check", func() []string { return []string{"check"} }},
		{"close", func() []string {
			k := "none"
			if rng.Intn(2) == 0 {
				k = "Learned " + phrase(5)
			}
			return []string{"close", "--summary", "Did " + phrase(4), "--knowledge", k}
		}},
		{"handover", func() []string {
			return []string{"close", "--summary", phrase(3), "--knowledge", "none", "--status", "in_progress"}
		}},
		{"review", func() []string { return []string{"review", "pass", "--findings", fmt.Sprint(rng.Intn(2))} }},
		{"adr", func() []string { return []string{"adr", "new", "Use " + phrase(2)} }},
		{"switch", func() []string {
			return []string{"switch", []string{"codex", "claude", "opencode", "x"}[rng.Intn(4)], "--print"}
		}},
		{"brief", func() []string { return []string{"brief", "--budget", fmt.Sprint(500 + rng.Intn(4000))} }},
		{"edit-code", nil},
	}
	for step := 0; step < steps; step++ {
		op := ops[rng.Intn(len(ops))]
		switch op.name {
		case "toggle-ok":
			p := filepath.Join(dir, "ok.txt")
			if _, err := os.Stat(p); err == nil {
				os.Remove(p)
			} else {
				os.WriteFile(p, []byte("1"), 0o644)
			}
			continue
		case "edit-code":
			write(t, dir, fmt.Sprintf("src/f%d.txt", rng.Intn(5)), phrase(8))
			continue
		}
		args := op.args()
		r := aitk(t, dir, args...) // fails the test on invalid JSON or envelope
		counts[op.name]++
		if !validExit[r.code] {
			t.Fatalf("step %d %v: exit %d not in spec", step, args, r.code)
		}
		if r.code != 0 && errCode(r) == "" {
			t.Fatalf("step %d %v: failure without error code", step, args)
		}
		if r.code == 0 {
			switch op.name {
			case "task-new":
				ids = append(ids, data(r)["id"].(string))
			case "knowledge":
				added = append(added, strings.TrimSpace(args[2]))
			case "close":
				if args[len(args)-1] != "none" && data(r)["knowledge_recorded"] == true {
					added = append(added, args[len(args)-1])
				}
			}
		}
		if step%100 == 99 {
			assertHealthy(t, dir, step, added)
		}
	}
	assertHealthy(t, dir, steps, added)
	t.Logf("ops: %v; tasks: %d; knowledge recorded: %d", counts, len(ids), len(added))
}

func assertHealthy(t *testing.T, dir string, step int, added []string) {
	t.Helper()
	d := aitk(t, dir, "doctor")
	if d.code != 0 {
		t.Fatalf("after step %d doctor reports errors:\n%s", step, d.stdout)
	}
	var all strings.Builder
	filepath.Walk(filepath.Join(dir, "docs/ai/knowledge"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, _ := os.ReadFile(p)
			for _, e := range knowledge.Parse(string(b), "", "", "") {
				all.WriteString(knowledge.Normalize(e.Text) + "\n")
			}
		}
		return nil
	})
	have := all.String()
	for _, text := range added {
		if !strings.Contains(have, knowledge.Normalize(text)) {
			t.Fatalf("after step %d knowledge lost: %q", step, text)
		}
	}
}
