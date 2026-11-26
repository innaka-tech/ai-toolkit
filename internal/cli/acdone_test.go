package cli

import "testing"

func criteriaDone(t *testing.T, dir, id string) []bool {
	t.Helper()
	r := aitk(t, dir, "task", "show", id)
	mustOK(t, r)
	var out []bool
	for _, c := range data(r)["criteria"].([]any) {
		out = append(out, c.(map[string]any)["done"].(bool))
	}
	return out
}

func TestTaskUpdateACDoneAccumulatesRepeatedFlags(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	id := data(aitk(t, dir, "task", "new", "Three", "--ac", "a", "--ac", "b", "--ac", "c", "--ac", "d"))["id"].(string)

	mustOK(t, aitk(t, dir, "task", "update", id, "--ac-done", "1", "--ac-done", "2"))
	if got := criteriaDone(t, dir, id); !got[0] || !got[1] || got[2] || got[3] {
		t.Fatalf("repeated --ac-done must mark every value, got %v", got)
	}
	mustOK(t, aitk(t, dir, "task", "update", id, "--ac-done", "3 4"))
	if got := criteriaDone(t, dir, id); !got[2] || !got[3] {
		t.Fatalf("space-separated form must keep working, got %v", got)
	}

	id2 := data(aitk(t, dir, "task", "new", "Two", "--ac", "a", "--ac", "b"))["id"].(string)
	mustOK(t, aitk(t, dir, "task", "update", id2, "--ac-done", "1,2"))
	if got := criteriaDone(t, dir, id2); !got[0] || !got[1] {
		t.Fatalf("comma form must keep working, got %v", got)
	}

	id3 := data(aitk(t, dir, "task", "new", "Bad", "--ac", "a", "--ac", "b"))["id"].(string)
	expect(t, aitk(t, dir, "task", "update", id3, "--ac-done", "1", "--ac-done", "x"), 2, "E_USAGE")
	if got := criteriaDone(t, dir, id3); got[0] {
		t.Fatalf("a rejected update must change nothing, got %v", got)
	}
	if r := aitk(t, dir, "task", "update", id3, "--ac-done", "1", "--ac-done", "9"); r.code == 0 {
		t.Fatalf("out-of-range criterion must fail: %s", r.stdout)
	}
	if got := criteriaDone(t, dir, id3); got[0] {
		t.Fatalf("a rejected update must change nothing, got %v", got)
	}
}
