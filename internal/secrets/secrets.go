// Package secrets detects credentials in added lines (a focused subset of gitleaks rules).
package secrets

import (
	"math"
	"regexp"
	"strings"
)

// Finding is one detected secret; Match is redacted.
type Finding struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Rule  string `json:"rule"`
	Match string `json:"match"`
}

type rule struct {
	name    string
	re      *regexp.Regexp
	entropy float64 // minimum Shannon entropy of the captured value (0 = no check)
}

var rules = []rule{
	{"private-key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP |SSH2 |ENCRYPTED )*PRIVATE KEY(?: BLOCK)?-----`), 0},
	{"aws-access-key", regexp.MustCompile(`\b((?:AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16})\b`), 0},
	{"github-token", regexp.MustCompile(`\b((?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36,})\b`), 0},
	{"github-fine-grained-token", regexp.MustCompile(`\b(github_pat_[A-Za-z0-9_]{60,})\b`), 0},
	{"gitlab-token", regexp.MustCompile(`\b(glpat-[A-Za-z0-9_-]{20,})\b`), 0},
	{"slack-token", regexp.MustCompile(`\b(xox[baprs]-[A-Za-z0-9-]{10,})\b`), 0},
	{"stripe-live-key", regexp.MustCompile(`\b([rs]k_live_[0-9A-Za-z]{24,})\b`), 0},
	{"google-api-key", regexp.MustCompile(`\b(AIza[0-9A-Za-z_-]{35})\b`), 0},
	{"anthropic-api-key", regexp.MustCompile(`\b(sk-ant-[A-Za-z0-9_-]{32,})`), 0},
	{"openai-api-key", regexp.MustCompile(`\b(sk-(?:proj-|svcacct-)?[A-Za-z0-9_-]{40,})`), 3.5},
	{"cloudflare-api-token", regexp.MustCompile(`\b(cfat_[A-Za-z0-9]{40,})\b`), 0},
	{"jwt", regexp.MustCompile(`\b(eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,})\b`), 0},
	// key = "value": the key ends with a secret word (an environment suffix such as _KEY or _PROD
	// may follow), and the quoted value is long and random enough.
	{"generic-secret-assignment", regexp.MustCompile(`(?i)\b[\w.-]*(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|client[_-]?secret)(?:[_-](?:key|value|prod|production|live|staging|dev))?\b["']?\s*[:=]\s*["']([^"'\s$<>{}]{12,})["']`), 3.3},
	// .env and shell lines: UPPER_CASE_KEY=value without quotes.
	{"generic-secret-assignment", regexp.MustCompile(`^\s*(?:export\s+)?[A-Z0-9_]*(?:PASSWORD|PASSWD|PWD|SECRET|TOKEN|API_?KEY|ACCESS_?KEY|CLIENT_?SECRET)(?:_(?:KEY|VALUE|PROD|PRODUCTION|LIVE|STAGING|DEV))?=([^"'\s$<>{}#]{12,})\s*(?:#.*)?$`), 3.3},
	{"url-credentials", regexp.MustCompile(`\b[a-z][a-z0-9+.-]*://([^/\s:@"']+):([^/\s:@"']{3,})@`), 0},
}

// Allow markers: lines containing these are skipped.
var allow = []string{"aitk:allow-secret", "gitleaks:allow"}

// ScanLine checks one line.
func ScanLine(file string, n int, line string) []Finding {
	for _, a := range allow {
		if strings.Contains(line, a) {
			return nil
		}
	}
	var out []Finding
	for _, r := range rules {
		for _, m := range r.re.FindAllStringSubmatch(line, -1) {
			v := m[0]
			for _, g := range m[1:] { // the first group that matched is the secret
				if g != "" {
					v = g
					break
				}
			}
			if r.name == "url-credentials" {
				if user := m[1]; len(m) > 2 {
					v = m[2]
					if strings.EqualFold(user, v) || weakTestPassword(v) {
						continue // postgres://postgres:postgres@localhost and friends
					}
				}
			}
			if r.entropy > 0 && Entropy(v) < r.entropy {
				continue
			}
			if placeholder(v) {
				continue
			}
			if r.name == "openai-api-key" && strings.HasPrefix(v, "sk-ant-") {
				continue // the anthropic rule reports it
			}
			if (r.name == "generic-secret-assignment" || r.name == "url-credentials") && codeReference(v) {
				continue
			}
			out = append(out, Finding{File: file, Line: n, Rule: r.name, Match: redact(v)})
		}
	}
	return out
}

// ScanDiff scans added lines of a unified diff (git diff -U0).
func ScanDiff(diff string) []Finding {
	var out []Finding
	file, n := "", 0
	header := false // between "diff --git" and the first "@@": file names live here, not in hunks
	hunk := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)`)
	for _, l := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			header, file = true, ""
		case header && strings.HasPrefix(l, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(l, "+++ "), "b/")
		case header && !strings.HasPrefix(l, "@@"):
			// ---, index, mode, rename lines
		case strings.HasPrefix(l, "@@"):
			header = false
			if m := hunk.FindStringSubmatch(l); m != nil {
				n = atoi(m[1])
			}
		case strings.HasPrefix(l, "+"):
			out = append(out, ScanLine(file, n, l[1:])...)
			n++
		case strings.HasPrefix(l, " "):
			n++
		}
	}
	return out
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// Entropy is the Shannon entropy in bits per character.
func Entropy(s string) float64 {
	if s == "" {
		return 0
	}
	freq := map[rune]float64{}
	for _, r := range s {
		freq[r]++
	}
	var h float64
	n := float64(len([]rune(s)))
	for _, c := range freq {
		p := c / n
		h -= p * math.Log2(p)
	}
	return h
}

var snakeWords = regexp.MustCompile(`^[a-z]+(?:[_-][a-z]+)+$`)

var identPath = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)+$`)

// codeReference reports values that are code, not secrets: os.environ.TOKEN, config.api_key,
// getToken(, password words in URLs like "user:password@".
func codeReference(v string) bool {
	if identPath.MatchString(v) || strings.ContainsAny(v, "(/+*&") || strings.HasPrefix(strings.ToLower(v), "http") {
		return true
	}
	if !strings.ContainsAny(v, "0123456789") && strings.ToUpper(v) == v && strings.Contains(v, "_") {
		return true // another variable: PASSWORD_RESET_DAYS
	}
	if snakeWords.MatchString(v) || strings.HasPrefix(strings.ToLower(v), "urn:") {
		return true // an identifier or protocol constant: "expired_token", "urn:ietf:params:…"
	}
	switch strings.ToLower(v) {
	case "password", "pass", "passwd", "secret", "token", "pwd":
		return true
	}
	return false
}

func weakTestPassword(v string) bool {
	switch strings.ToLower(v) {
	case "postgres", "password", "pass", "root", "admin", "test", "secret", "user", "guest", "mysql", "redis", "changeme":
		return true
	}
	return false
}

func placeholder(v string) bool {
	l := strings.ToLower(v)
	for _, p := range []string{"example", "changeme", "placeholder", "your_", "your-", "xxxxxxxx", "dummy", "redacted", "replace_me", "replace-me", "<", "${"} {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}

func redact(v string) string {
	r := []rune(v)
	if len(r) <= 8 {
		return strings.Repeat("*", len(r))
	}
	return string(r[:4]) + strings.Repeat("*", 8) + string(r[len(r)-2:])
}
