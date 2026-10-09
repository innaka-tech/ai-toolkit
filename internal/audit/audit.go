// Package audit runs security scanners (dependency vulnerabilities, static analysis, secrets)
// for the project aitk manages.
package audit

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/v2/internal/checkrun"
	"github.com/innaka-tech/ai-toolkit/v2/internal/fsx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/gitx"
	"github.com/innaka-tech/ai-toolkit/v2/internal/project"
	"github.com/innaka-tech/ai-toolkit/v2/internal/secrets"
	"github.com/innaka-tech/ai-toolkit/v2/internal/task"
)

// Detect returns the scanners to run: [security].scanners when configured, else the ones
// installed and relevant to this repository. The built-in secret scan always runs.
func Detect(p *project.Project) (scanners []project.Scanner, missing []string) {
	if len(p.Config.Security.Scanners) > 0 {
		return p.Config.Security.Scanners, nil
	}
	has := func(f string) bool { return fsx.Exists(p.Path(f)) }
	bin := func(b string) bool { _, err := exec.LookPath(b); return err == nil }
	add := func(name, cmd string) { scanners = append(scanners, project.Scanner{Name: name, Cmd: cmd}) }
	osv := bin("osv-scanner")
	if osv {
		add("osv-scanner (dependencies)", "osv-scanner scan source --recursive .")
	}
	type eco struct {
		files []string
		bin   string
		cmd   string
		name  string
	}
	for _, e := range []eco{
		{[]string{"package-lock.json"}, "npm", "npm audit --audit-level=high", "npm audit"},
		{[]string{"pnpm-lock.yaml"}, "pnpm", "pnpm audit --audit-level high", "pnpm audit"},
		{[]string{"go.mod"}, "govulncheck", "govulncheck ./...", "govulncheck"},
		{[]string{"requirements.txt", "pyproject.toml"}, "pip-audit", "pip-audit", "pip-audit"},
		{[]string{"composer.lock"}, "composer", "composer audit", "composer audit"},
		{[]string{"Cargo.lock"}, "cargo-audit", "cargo audit", "cargo audit"},
	} {
		relevant := false
		for _, f := range e.files {
			relevant = relevant || has(f)
		}
		if !relevant {
			continue
		}
		if bin(e.bin) {
			add(e.name, e.cmd)
		} else if !osv {
			missing = append(missing, e.name+" (or osv-scanner)")
		}
	}
	if bin("semgrep") {
		add("semgrep OWASP Top 10 (static analysis)", "semgrep scan --config p/owasp-top-ten --error --quiet --metrics=off")
	}
	return scanners, missing
}

// Run executes the scanners and the built-in secret scan.
func Run(p *project.Project, stream bool) (*task.Audit, []string) {
	scanners, missing := Detect(p)
	a := &task.Audit{At: task.Now(), Passed: true}
	for _, s := range scanners {
		r := checkrun.Run(p.Root, s.Cmd, 15*time.Minute, stream)
		a.Results = append(a.Results, task.AuditResult{Name: s.Name, ExitCode: r.ExitCode, Summary: checkrun.Summary(strings.TrimSpace(r.Output), 1500)})
		if r.ExitCode != 0 {
			a.Passed = false
		}
	}
	fs := ScanTrackedFiles(p)
	res := task.AuditResult{Name: "aitk secret scan (tracked files)"}
	if len(fs) > 0 {
		res.ExitCode = 1
		var lines []string
		for i, f := range fs {
			if i == 20 {
				lines = append(lines, "…")
				break
			}
			lines = append(lines, f.File+":"+itoa(f.Line)+" "+f.Rule+" "+f.Match)
		}
		res.Summary = strings.Join(lines, "\n")
		a.Passed = false
	}
	a.Results = append(a.Results, res)
	var warns []string
	for _, m := range missing {
		warns = append(warns, "no scanner installed for "+m+"; dependency vulnerabilities were not checked")
	}
	return a, warns
}

// ScanTrackedFiles scans every tracked text file for credentials.
func ScanTrackedFiles(p *project.Project) []secrets.Finding {
	out, _ := gitx.RunRaw(p.Root, "ls-files", "-z")
	var fs []secrets.Finding
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" || strings.HasPrefix(rel, "docs/ai/_legacy/") {
			continue
		}
		info, err := os.Stat(p.Path(rel))
		if err != nil || info.Size() > 2<<20 || info.IsDir() {
			continue
		}
		b, err := os.ReadFile(p.Path(rel))
		if err != nil || isBinary(b) {
			continue
		}
		for i, line := range strings.Split(string(b), "\n") {
			fs = append(fs, secrets.ScanLine(rel, i+1, line)...)
		}
	}
	return fs
}

func isBinary(b []byte) bool {
	n := len(b)
	if n > 8000 {
		n = 8000
	}
	for _, c := range b[:n] {
		if c == 0 {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
