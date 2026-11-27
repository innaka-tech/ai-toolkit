package cli

import (
	"strings"
	"testing"
)

func briefMD(t *testing.T, dir string) (string, map[string]any) {
	t.Helper()
	r := aitk(t, dir, "brief")
	mustOK(t, r)
	d := data(r)
	return d["markdown"].(string), d
}

func personaNames(d map[string]any) []string {
	var out []string
	ps, _ := d["personas"].([]any)
	for _, p := range ps {
		out = append(out, p.(map[string]any)["name"].(string))
	}
	return out
}

func has(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestAssistantCharacterAndTalkFromConfigAndEnv(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	md, _ := briefMD(t, dir)
	if !strings.Contains(md, "Smallest change that fully works") || strings.Contains(md, "Talk caveman") {
		t.Fatalf("defaults must be character full, talk normal:\n%s", md)
	}

	write(t, dir, "aitk.toml", read(t, dir, "aitk.toml")+"\n[assistant]\ncharacter = \"ultra\"\ntalk = \"caveman\"\n")
	md, _ = briefMD(t, dir)
	if !strings.Contains(md, "Character ultra") || !strings.Contains(md, "Talk caveman") || !strings.Contains(md, "Never shorten code") {
		t.Fatalf("[assistant] must reach the brief rules:\n%s", md)
	}

	t.Setenv("AITK_TALK", "normal")
	t.Setenv("AITK_CHARACTER", "off")
	md, _ = briefMD(t, dir)
	if strings.Contains(md, "Talk caveman") || strings.Contains(md, "Character ultra") || strings.Contains(md, "Smallest change") {
		t.Fatalf("AITK_TALK / AITK_CHARACTER must override aitk.toml for one person:\n%s", md)
	}

	t.Setenv("AITK_TALK", "shouting") // invalid env values fall back to the config instead of failing
	md, _ = briefMD(t, dir)
	if !strings.Contains(md, "Talk caveman") {
		t.Fatalf("an invalid AITK_TALK must be ignored:\n%s", md)
	}

	write(t, dir, "aitk.toml", strings.Replace(read(t, dir, "aitk.toml"), `talk = "caveman"`, `talk = "loud"`, 1))
	expect(t, aitk(t, dir, "brief"), 3, "E_SCHEMA")
}

func TestAssistantPicksPersonasForTheActiveTask(t *testing.T) {
	dir := repo(t)
	initRepo(t, dir)
	_, d := briefMD(t, dir)
	if got := personaNames(d); len(got) != 1 || got[0] != "aitk-team" {
		t.Fatalf("no active task: want aitk-team, got %v", got)
	}

	mustOK(t, aitk(t, dir, "task", "new", "Add shipment status", "--tag", "bug", "--ac", "Given x, when y, then z", "--start"))
	write(t, dir, "db/migrations/20261127_add_status.sql", "ALTER TABLE shipment ADD status text;\n")
	write(t, dir, "web/Status.tsx", "export const S = () => null\n")
	write(t, dir, "Dockerfile", "FROM scratch\n")
	write(t, dir, "internal/status.go", "package internal\n")
	md, d := briefMD(t, dir)
	got := personaNames(d)
	for _, want := range []string{"aitk-fix", "aitk-dba", "aitk-ux", "aitk-devops", "aitk-qa"} {
		if !has(got, want) {
			t.Fatalf("want %s among %v\n%s", want, got, md)
		}
	}
	if !strings.Contains(md, "aitk prompt aitk-dba fix") || !strings.Contains(md, "aitk standards database") {
		t.Fatalf("the brief must say how to run each persona and which standards apply:\n%s", md)
	}

	write(t, dir, "internal/status_test.go", "package internal\n")
	if _, d = briefMD(t, dir); has(personaNames(d), "aitk-qa") {
		t.Fatalf("a changed test file must drop aitk-qa: %v", personaNames(d))
	}

	write(t, dir, "aitk.toml", read(t, dir, "aitk.toml")+"\n[assistant]\nauto_persona = false\n")
	if md, d = briefMD(t, dir); len(personaNames(d)) != 0 || strings.Contains(md, "## Personas") {
		t.Fatalf("auto_persona = false must list no personas: %v", personaNames(d))
	}
}
