package audit

import "testing"

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.10.0", "1.9.2", 1}, {"1.2.0", "1.2.0", 0}, {"v1.2.3", "1.2.3", 0}, {"2.0.0", "10.0.0", -1},
		{"1.2.0-rc.1", "1.2.0", -1}, {"1.2.0", "1.2.0-rc.1", 1}, {"1.2", "1.2.1", -1}, {"", "0.0.1", -1},
		{"7.5.21", "7.5.3", 1}, {"2.0.0-beta.10", "2.0.0-beta.9", 1},
		{"2.0.0rc1", "2.0.0", -1}, {"1.10a1", "1.9", 1}, {"1.0.0+build.5", "1.0.0", 0}, {"2.17.1.Final", "2.17.1", 0},
		{"1.2.post1", "1.2", 1}, {"1.0.0-alpha", "1.0.0-beta", -1}, {"1.2", "1.2.0", 0}, {"3.0.0-rc.1", "2.9.9", 1},
	} {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// One package installed in two versions (two lockfiles) is one entry with both versions,
// each with its own fixed version.
func TestParseOSVGroupsVersions(t *testing.T) {
	js := `{"results":[
 {"source":{"path":"package-lock.json"},"packages":[{"package":{"name":"form-data","version":"4.0.5","ecosystem":"npm"},
  "vulnerabilities":[{"id":"GHSA-1","affected":[{"package":{"name":"form-data"},"ranges":[{"events":[{"introduced":"4.0.0"},{"fixed":"4.0.6"}]},{"events":[{"introduced":"2.0.0"},{"fixed":"2.5.4"}]}]}]}],
  "groups":[{"ids":["GHSA-1"],"max_severity":"9.1"}]}]},
 {"source":{"path":"web/package-lock.json"},"packages":[{"package":{"name":"form-data","version":"2.5.1","ecosystem":"npm"},
  "vulnerabilities":[{"id":"GHSA-1","affected":[{"package":{"name":"form-data"},"ranges":[{"events":[{"introduced":"4.0.0"},{"fixed":"4.0.6"}]},{"events":[{"introduced":"2.0.0"},{"fixed":"2.5.4"}]}]}]}]}]}]}`
	pkgs, err := ParseOSV([]byte(js))
	if err != nil || len(pkgs) != 1 {
		t.Fatalf("want one package, got %+v %v", pkgs, err)
	}
	p := pkgs[0]
	if len(p.Versions) != 2 || len(p.Advisories) != 1 || p.MaxSeverity != 9.1 {
		t.Fatalf("unexpected grouping: %+v", p)
	}
	if v := p.Versions[0]; v.Version != "4.0.5" || v.FixedIn != "4.0.6" || v.Sources[0] != "package-lock.json" {
		t.Fatalf("4.x line: %+v", v)
	}
	if v := p.Versions[1]; v.Version != "2.5.1" || v.FixedIn != "2.5.4" || v.Sources[0] != "web/package-lock.json" {
		t.Fatalf("2.x line must get its own fix: %+v", v)
	}
}
