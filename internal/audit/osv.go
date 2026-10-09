package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// VulnPackage is one dependency with known vulnerabilities, as reported by osv-scanner,
// with every vulnerable version found in the repository.
type VulnPackage struct {
	Ecosystem   string        `json:"ecosystem"`
	Name        string        `json:"name"`
	Versions    []VulnVersion `json:"versions"`
	Advisories  []string      `json:"advisories"`
	MaxSeverity float64       `json:"max_severity,omitempty"` // CVSS
}

// VulnVersion is one installed version of a vulnerable package.
type VulnVersion struct {
	Version string   `json:"version"`
	FixedIn string   `json:"fixed_in,omitempty"` // lowest version that fixes every advisory, when known
	Sources []string `json:"sources"`
}

type osvOutput struct {
	Results []struct {
		Source struct {
			Path string `json:"path"`
		} `json:"source"`
		Packages []struct {
			Package struct {
				Name      string `json:"name"`
				Version   string `json:"version"`
				Ecosystem string `json:"ecosystem"`
			} `json:"package"`
			Vulnerabilities []struct {
				ID       string `json:"id"`
				Affected []struct {
					Package struct {
						Name      string `json:"name"`
						Ecosystem string `json:"ecosystem"`
					} `json:"package"`
					Ranges []struct {
						Type   string `json:"type"`
						Events []struct {
							Fixed string `json:"fixed"`
						} `json:"events"`
					} `json:"ranges"`
				} `json:"affected"`
			} `json:"vulnerabilities"`
			Groups []struct {
				IDs         []string `json:"ids"`
				MaxSeverity string   `json:"max_severity"`
			} `json:"groups"`
		} `json:"packages"`
	} `json:"results"`
}

// OSVPackages runs osv-scanner with JSON output in root and groups advisories by package.
func OSVPackages(root string) ([]VulnPackage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	c := exec.CommandContext(ctx, "osv-scanner", "scan", "source", "--recursive", "--format", "json", ".")
	c.Dir = root
	out, err := c.Output()
	if ee, ok := err.(*exec.ExitError); ok && (ee.ExitCode() == 1 || ee.ExitCode() == osvNoPackages) {
		err = nil // 1: vulnerabilities found; 128: nothing to scan
	}
	if err != nil {
		return nil, fmt.Errorf("osv-scanner: %w", err)
	}
	pkgs, err := ParseOSV(out)
	for i := range pkgs {
		for j := range pkgs[i].Versions {
			for k, src := range pkgs[i].Versions[j].Sources {
				if rel, err := filepath.Rel(root, src); err == nil && !strings.HasPrefix(rel, "..") {
					pkgs[i].Versions[j].Sources[k] = filepath.ToSlash(rel)
				}
			}
		}
	}
	return pkgs, err
}

// ParseOSV groups osv-scanner JSON output by package.
func ParseOSV(out []byte) ([]VulnPackage, error) {
	var o osvOutput
	if len(strings.TrimSpace(string(out))) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(out, &o); err != nil {
		return nil, fmt.Errorf("osv-scanner output: %w", err)
	}
	byKey := map[string]*VulnPackage{}
	var keys []string
	for _, r := range o.Results {
		for _, pk := range r.Packages {
			if len(pk.Vulnerabilities) == 0 {
				continue
			}
			k := pk.Package.Ecosystem + "\x00" + pk.Package.Name
			vp := byKey[k]
			if vp == nil {
				vp = &VulnPackage{Ecosystem: pk.Package.Ecosystem, Name: pk.Package.Name}
				byKey[k] = vp
				keys = append(keys, k)
			}
			var vv *VulnVersion
			for i := range vp.Versions {
				if vp.Versions[i].Version == pk.Package.Version {
					vv = &vp.Versions[i]
				}
			}
			if vv == nil {
				vp.Versions = append(vp.Versions, VulnVersion{Version: pk.Package.Version})
				vv = &vp.Versions[len(vp.Versions)-1]
			}
			if !has(vv.Sources, r.Source.Path) {
				vv.Sources = append(vv.Sources, r.Source.Path)
			}
			for _, g := range pk.Groups {
				if s, err := strconv.ParseFloat(g.MaxSeverity, 64); err == nil && s > vp.MaxSeverity {
					vp.MaxSeverity = s
				}
			}
			for _, v := range pk.Vulnerabilities {
				if !has(vp.Advisories, v.ID) {
					vp.Advisories = append(vp.Advisories, v.ID)
				}
				// The fix for this advisory: its lowest fixed version above the installed one.
				fix := ""
				for _, a := range v.Affected {
					if a.Package.Name != pk.Package.Name || a.Package.Ecosystem != "" && !strings.EqualFold(a.Package.Ecosystem, pk.Package.Ecosystem) {
						continue
					}
					for _, rg := range a.Ranges {
						if strings.EqualFold(rg.Type, "GIT") {
							continue // commit hashes, not versions
						}
						for _, e := range rg.Events {
							if e.Fixed != "" && CompareVersions(e.Fixed, pk.Package.Version) > 0 && (fix == "" || CompareVersions(e.Fixed, fix) < 0) {
								fix = e.Fixed
							}
						}
					}
				}
				if fix != "" && CompareVersions(fix, vv.FixedIn) > 0 {
					vv.FixedIn = fix
				}
			}
		}
	}
	out2 := make([]VulnPackage, 0, len(keys))
	for _, k := range keys {
		vp := byKey[k]
		sort.Strings(vp.Advisories)
		sort.SliceStable(vp.Versions, func(i, j int) bool { return CompareVersions(vp.Versions[i].Version, vp.Versions[j].Version) > 0 })
		for i := range vp.Versions {
			sort.Strings(vp.Versions[i].Sources)
		}
		out2 = append(out2, *vp)
	}
	sort.SliceStable(out2, func(i, j int) bool { return out2[i].MaxSeverity > out2[j].MaxSeverity })
	return out2, nil
}

// CompareVersions orders package versions across the common schemes: SemVer (build metadata
// ignored, pre-releases before the release), PEP 440 ("2.0.0rc1" < "2.0.0", "1.10a1" > "1.9"),
// and Maven qualifiers (".Final" equals the release). "" is lower than any version.
func CompareVersions(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" || b == "" {
		if a == "" {
			return -1
		}
		return 1
	}
	ta, tb := versionTokens(a), versionTokens(b)
	for i := 0; ; i++ {
		switch {
		case i >= len(ta) && i >= len(tb):
			return 0
		case i >= len(ta):
			return -tail(tb[i:])
		case i >= len(tb):
			return tail(ta[i:])
		}
		x, y := ta[i], tb[i]
		nx, ex := strconv.Atoi(x)
		ny, ey := strconv.Atoi(y)
		switch {
		case ex == nil && ey == nil:
			if nx != ny {
				return sign(nx - ny)
			}
		case ex == nil: // number > qualifier: 1.0.1 > 1.0.rc1
			return 1
		case ey == nil:
			return -1
		default:
			if rx, ry := qualifierRank(x), qualifierRank(y); rx != ry {
				return sign(rx - ry)
			}
			if x != y {
				return strings.Compare(x, y)
			}
		}
	}
}

// tail compares the rest of the longer version with nothing: zeros do not count (1.2 == 1.2.0).
func tail(rest []string) int {
	for _, t := range rest {
		if t != "0" {
			return tailSign(t)
		}
	}
	return 0
}

// tailSign: how the longer version compares when the other one has run out at token t.
// "1.2.1" > "1.2"; "1.2rc1" < "1.2"; "1.2.post1" > "1.2".
func tailSign(t string) int {
	if _, err := strconv.Atoi(t); err == nil {
		return 1
	}
	if qualifierRank(t) > qualifierRank("") {
		return 1
	}
	return -1
}

func qualifierRank(q string) int {
	switch q {
	case "dev", "snapshot":
		return 0
	case "a", "alpha":
		return 1
	case "b", "beta":
		return 2
	case "m", "milestone":
		return 3
	case "pre", "preview", "c", "rc", "cr":
		return 4
	case "": // the release itself
		return 5
	case "post", "p", "sp", "patch":
		return 6
	}
	return 4 // unknown qualifiers count as pre-releases
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// versionTokens splits a version into numbers and words, dropping separators, a leading "v",
// build metadata, and qualifiers that mean "the release" (final, ga, release).
func versionTokens(v string) []string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	var out []string
	cur, digit := "", false
	flush := func() {
		if cur != "" {
			out = append(out, cur)
		}
		cur = ""
	}
	for _, r := range v {
		isDigit := r >= '0' && r <= '9'
		isAlpha := r >= 'a' && r <= 'z'
		switch {
		case !isDigit && !isAlpha:
			flush()
		case cur != "" && isDigit != digit:
			flush()
			cur, digit = string(r), isDigit
		default:
			cur += string(r)
			digit = isDigit
		}
	}
	flush()
	for len(out) > 0 {
		switch out[len(out)-1] {
		case "final", "ga", "release":
			out = out[:len(out)-1]
			continue
		}
		break
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
