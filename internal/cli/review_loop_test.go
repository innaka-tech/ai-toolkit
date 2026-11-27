package cli

import (
	"fmt"
	"strings"
	"testing"
)

// standardChange touches enough files that the task is not lite.
func standardChange(t *testing.T, dir, tag string) {
	t.Helper()
	for i := 0; i < 5; i++ {
		write(t, dir, fmt.Sprintf("src/%s_%d.go", tag, i), "package src\n")
	}
	write(t, dir, "ok.txt", "1")
}

func status(t *testing.T, dir, id string) string {
	t.Helper()
	return data(aitk(t, dir, "task", "show", id))["task"].(map[string]any)["status"].(string)
}

func TestReviewUntilCleanBeforeDone(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir) // aitk init writes [quality] review = "standard"
	if !strings.Contains(read(t, dir, "aitk.toml"), `review = "standard"`) {
		t.Fatal("aitk init must turn on review until clean for new projects")
	}
	id := data(aitk(t, dir, "task", "new", "Shipment export", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	standardChange(t, dir, "a")
	mustOK(t, aitk(t, dir, "task", "update", id, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))

	r := aitk(t, dir, "close", "--summary", "export", "--knowledge", "none")
	mustOK(t, r)
	if st := status(t, dir, id); st != "in_review" || !strings.Contains(r.stdout, "review until clean") {
		t.Fatalf("a standard task without a clean review must wait in_review with the loop explained, got %s:\n%s", st, r.stdout)
	}
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "2"))
	mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
	if st := status(t, dir, id); st != "in_review" {
		t.Fatalf("a pass with findings must not finish the task, got %s", st)
	}
	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "0"))
	mustOK(t, aitk(t, dir, "close", "--summary", "export", "--knowledge", "none"))
	if st := status(t, dir, id); st != "done" {
		t.Fatalf("a clean last pass must let the task finish, got %s", st)
	}
}

func TestOneTaskAtATime(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	a := data(aitk(t, dir, "task", "new", "First", "--start"))["id"].(string)
	b := data(aitk(t, dir, "task", "new", "Second"))["id"].(string)

	expect(t, aitk(t, dir, "task", "start", b), 3, "E_TASK_ACTIVE")
	expect(t, aitk(t, dir, "task", "new", "Third", "--start"), 3, "E_TASK_ACTIVE")
	if r := aitk(t, dir, "task", "next", "--start"); r.code == 0 && !strings.Contains(r.stdout, a) {
		t.Fatalf("task next --start must resume the active task or refuse, never switch: %s", r.stdout)
	}
	if r := aitk(t, dir, "task", "start", b); !strings.Contains(r.stdout, "aitk task block "+a) {
		t.Fatalf("the refusal must say how to park the active task: %s", r.stdout)
	}

	mustOK(t, aitk(t, dir, "review", "pass", "--findings", "1")) // in_review still blocks a switch
	if st := status(t, dir, a); st != "in_review" {
		t.Fatalf("setup: want in_review, got %s", st)
	}
	expect(t, aitk(t, dir, "task", "start", b), 3, "E_TASK_ACTIVE")

	mustOK(t, aitk(t, dir, "task", "block", a, "--reason", "waiting for the carrier API"))
	mustOK(t, aitk(t, dir, "task", "start", b))

	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), `review = "standard"`, "review = \"standard\"\none_task = false", 1))
	mustOK(t, aitk(t, dir, "task", "start", a))
}

func TestReviewNotConvergingAndBackCompat(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	id := data(aitk(t, dir, "task", "new", "Hard one", "--start"))["id"].(string)
	for _, n := range []string{"2", "2", "3"} {
		mustOK(t, aitk(t, dir, "review", "pass", "--findings", n))
	}
	md, _ := briefMD(t, dir)
	if !strings.Contains(md, "Review is not converging (2, 2, 3") {
		t.Fatalf("passes that stop decreasing must make the brief say to stop and ask:\n%s", md)
	}
	mustOK(t, aitk(t, dir, "task", "block", id, "--reason", "split it"))

	// Projects without the setting keep the v2 behaviour: only strict work needs reviews.
	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), `review = "standard"`, "", 1))
	id2 := data(aitk(t, dir, "task", "new", "Old project task", "--ac", "Given x, when y, then z", "--start"))["id"].(string)
	standardChange(t, dir, "b")
	mustOK(t, aitk(t, dir, "task", "update", id2, "--ac-done", "1"))
	mustOK(t, aitk(t, dir, "check"))
	mustOK(t, aitk(t, dir, "close", "--summary", "x", "--knowledge", "none"))
	if st := status(t, dir, id2); st != "done" {
		t.Fatalf("without [quality] review a standard task must finish as before, got %s", st)
	}
}
