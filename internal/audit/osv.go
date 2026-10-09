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
					if a.Package.Name != pk.Package.Name {
						continue
					}
					for _, rg := range a.Ranges {
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

// CompareVersions compares dotted versions numerically segment by segment ("1.10.0" > "1.9.2").
// Pre-release suffixes compare as text after the numbers. "" is lower than any version.
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
	pa, pb := splitVersion(a), splitVersion(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y string
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		nx, ex := strconv.Atoi(x)
		ny, ey := strconv.Atoi(y)
		switch {
		case ex == nil && ey == nil:
			if nx != ny {
				if nx < ny {
					return -1
				}
				return 1
			}
		case x == "" || y == "":
			// "1.2" vs "1.2.0-rc": the shorter numeric version wins only against a pre-release
			if x == "" {
				if ey != nil {
					return 1
				}
				return -1
			}
			if ex != nil {
				return -1
			}
			return 1
		default:
			if x != y {
				if x < y {
					return -1
				}
				return 1
			}
		}
	}
	return 0
}

func splitVersion(v string) []string {
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	return strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' || r == '+' || r == '_' })
}

func has(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
