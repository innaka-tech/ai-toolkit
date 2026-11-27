package cli

// aitk-slop: ignore-file (fixtures below contain the patterns on purpose)

import (
	"strings"
	"testing"
)

func slopRules(r res) map[string][]string {
	out := map[string][]string{}
	items, _ := r.env["data"].([]any)
	for _, it := range items {
		m := it.(map[string]any)
		out[m["rule"].(string)] = append(out[m["rule"].(string)], m["file"].(string))
	}
	return out
}

func TestSlopFindsGenericAIOutputAndRespectsIgnores(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	write(t, dir, "docs/guide.md", "# 🚀 Getting started\n\nOur seamless, cutting-edge pipeline.\n\nCertainly! Here it is.\n\n```js\ntry { run() } catch (e) {}\n```\n\nLorem ipsum dolor.\n")
	write(t, dir, "app/job.py", "# Loop through the items\nfor i in items:\n    try:\n        run(i)\n    except: pass\n")
	write(t, dir, "app/ok.go", "package app\n\n// retry once: the carrier API drops the first call after idle\nfunc Ok() error { return nil }\n")
	write(t, dir, "app/allowed.go", "package app\n\nfunc F() { _ = err } // aitk-slop: ignore (err is always nil here)\n")
	write(t, dir, "app/fixture.go", "// aitk-slop: ignore-file\npackage app\n\n// increment the counter\n")
	write(t, dir, "notes.txt", "seamless delve lorem ipsum\n")

	r := aitk(t, dir, "slop")
	mustOK(t, r)
	got := slopRules(r)
	want := map[string]string{"emoji-heading": "docs/guide.md", "filler-phrase": "docs/guide.md", "assistant-voice": "docs/guide.md",
		"placeholder": "docs/guide.md", "restating-comment": "app/job.py"}
	for rule, file := range want {
		if !has(got[rule], file) {
			t.Fatalf("want %s in %s, got %v", rule, file, got)
		}
	}
	if sw := got["swallowed-error"]; !has(sw, "app/job.py") || !has(sw, "docs/guide.md") {
		t.Fatalf("swallowed errors in code and in fenced markdown code must be found: %v", sw)
	}
	for rule, files := range got {
		for _, f := range files {
			if f == "app/ok.go" || f == "app/allowed.go" || f == "app/fixture.go" || f == "notes.txt" {
				t.Fatalf("%s flagged in %s: clean files, ignored lines, ignored files and non-code files must not be reported", rule, f)
			}
		}
	}

	only := aitk(t, dir, "slop", "app/ok.go")
	mustOK(t, only)
	if list, ok := only.env["data"].([]any); !ok || len(list) != 0 {
		t.Fatalf("a clean path must give an empty list (not null): %s", only.stdout)
	}
}

func TestBriefPicksDeslopAndCreative(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	mustOK(t, aitk(t, dir, "task", "new", "Explore a new picking flow", "--tag", "spike", "--start"))
	_, d := briefMD(t, dir)
	if got := personaNames(d); !has(got, "aitk-creative") || has(got, "aitk-deslop") {
		t.Fatalf("spike task without slop: want aitk-creative only, got %v", got)
	}
	write(t, dir, "docs/flow.md", "# Flow\n\nA seamless experience.\n")
	md, d := briefMD(t, dir)
	if !has(personaNames(d), "aitk-deslop") || !strings.Contains(md, "docs/flow.md:3 [filler-phrase]") || !strings.Contains(md, "aitk standards anti-slop") {
		t.Fatalf("slop in the change must bring aitk-deslop with its first hit and the anti-slop standard:\n%s", md)
	}
}
