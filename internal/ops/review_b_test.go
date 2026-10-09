package ops

import "testing"

// Review B #2: IDs never collide for different packages, and recurrence suffixes stay exact.
func TestAuditIDsAreDistinctAndStable(t *testing.T) {
	a := auditID("VULN", "Maven-org.apache.logging.log4j:log4j-core")
	b := auditID("VULN", "Maven-org.apache.logging.log4j:log4j-api")
	if a == b || len(a) > 28 || len(b) > 28 {
		t.Fatalf("collision or too long: %q %q", a, b)
	}
	if auditID("VULN", "npm-axios") != "VULN-npm-axios" || auditID("VULN", "npm-axios") != auditID("VULN", "npm-axios") {
		t.Fatal("short names stay readable and stable")
	}
	for _, c := range []struct {
		task, id string
		want     bool
	}{
		{"VULN-npm-semver", "VULN-npm-semver", true}, {"VULN-npm-semver-2", "VULN-npm-semver", true},
		{"VULN-npm-semver-regex", "VULN-npm-semver", false}, {"VULN-npm-semver-", "VULN-npm-semver", false},
	} {
		if got := sameFinding(c.task, c.id); got != c.want {
			t.Errorf("sameFinding(%q, %q) = %v", c.task, c.id, got)
		}
	}
}

func TestTitleMatchesOldImports(t *testing.T) {
	if !titleMatches("Add cart model in src/cart.py", "T004 [US1] Add cart model in src/cart.py") {
		t.Fatal("markers must be ignored")
	}
	if !titleMatches("Very long…", "Very long title that was truncated") || titleMatches("Other", "T001 Something") {
		t.Fatal("truncated titles match by prefix only")
	}
}
