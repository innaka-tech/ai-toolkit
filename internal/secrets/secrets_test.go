package secrets

import (
	"strings"
	"testing"
)

// fake builds token-shaped test data at runtime so no literal secret lives in the source
// (repository secret scanners would block the push otherwise).
func fake(prefix, body string) string { return prefix + body }

func TestRules(t *testing.T) {
	pos := map[string]string{
		"private-key":               "-----BEGIN OPENSSH " + "PRIVATE KEY-----",
		"aws-access-key":            "aws_key = " + fake("AK"+"IA", "4QZBQ7XKZJ2MNPLR"),
		"github-token":              "token: " + fake("gh"+"p_", "aB3dE5fG7hI9jK1lM3nO5pQ7rS9tU1vW3xY5"),
		"slack-token":               "SLACK=" + fake("xo"+"xb-", "123456789012-abcdefghijkl"),
		"stripe-live-key":           `stripe("` + fake("sk"+"_live_", "4eC39HqLyjWDarjtT1zdp7dc") + `")`,
		"google-api-key":            "key=" + fake("AI"+"za", "SyD3x7Kq9Lm2Np4Rs6Tv8Wx0Yz1Ab3Cd5Ef"),
		"anthropic-api-key":         "ANTHROPIC_API_KEY=" + fake("sk-"+"ant-", "api03-Zq8xW2vY4uT6sR8pN0mL2kJ4hG6fD8sA0qW"), // aitk:allow-secret (fake fixture)
		"cloudflare-api-token":      "Authorization: Bearer " + fake("cf"+"at_", "Qx7Lm2Np4Rs6Tv8Wx0Yz1Ab3Cd5EfGh7Jk9Mn2Pq4"),
		"jwt":                       fake("ey"+"J", "hbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"),
		"generic-secret-assignment": `DB_PASSWORD="k8#Vq2!xR9zL4mPw"`, // aitk:allow-secret (fake fixture)
	}
	for rule, line := range pos {
		fs := ScanLine("f", 1, line)
		if len(fs) == 0 || fs[0].Rule != rule {
			t.Errorf("%s not detected in %q: %v", rule, line, fs)
		}
		for _, f := range fs {
			if len(f.Match) > 16 || f.Match == line {
				t.Errorf("match not redacted: %q", f.Match)
			}
		}
	}
	neg := []string{
		`password = os.Getenv("DB_PASSWORD")`,
		`token: "${GITHUB_TOKEN}"`,
		`api_key = "your_api_key_here"`,
		`secret = "aaaaaaaaaaaaaaaa"`,
		fake("AK"+"IA", "4QZBQ7XKZJ2MNPLR") + " # aitk:allow-secret",
		`See docs at https://example.com/sk-ant-docs`,
		`const placeholder = "REPLACE_ME"`,
	}
	for _, line := range neg {
		if fs := ScanLine("f", 1, line); len(fs) > 0 {
			t.Errorf("false positive in %q: %v", line, fs)
		}
	}
}

// Bug hunt: unquoted .env values, keyword suffixes, URL credentials, SSH2 keys; code references
// and duplicate reports are not findings.
func TestMoreRules(t *testing.T) {
	for line, rule := range map[string]string{
		"DB_PASSWORD=Hj8kLm2nPq9rSt4v":                   "generic-secret-assignment", // gitleaks:allow aitk:allow-secret
		"export API_TOKEN=Zq8xW2vY4uT6sR8pN0mL":          "generic-secret-assignment", // gitleaks:allow aitk:allow-secret
		`SECRET_KEY = "Qw3rTy7uI9oP1aS5dF"`:              "generic-secret-assignment", // gitleaks:allow aitk:allow-secret
		`PASSWORD_PROD="Mn4bV6cX8zL2kJ5h"`:               "generic-secret-assignment", // gitleaks:allow aitk:allow-secret
		"postgres://admin:S3cr3tPw@db.internal:5432/app": "url-credentials",           // gitleaks:allow aitk:allow-secret
		"-----BEGIN SSH2 ENCRYPTED PRIVATE KEY-----":     "private-key",               // gitleaks:allow aitk:allow-secret
	} {
		fs := ScanLine("f", 1, line)
		if len(fs) != 1 || fs[0].Rule != rule {
			t.Errorf("%q: want one %s finding, got %v", line, rule, fs)
		}
	}
	for _, line := range []string{
		"token = config.api.token",
		"password: os.environ.get(\"DB_PASSWORD\")",
		"https://user:password@example.com",
	} {
		if fs := ScanLine("f", 1, line); len(fs) > 0 {
			t.Errorf("false positive in %q: %v", line, fs)
		}
	}
	if fs := ScanLine("f", 1, "K="+fake("sk-"+"ant-", "api03-Zq8xW2vY4uT6sR8pN0mL2kJ4hG6fD8sA0qW")); len(fs) != 1 {
		t.Errorf("an anthropic key is one finding, got %v", fs)
	}
}

// Bug hunt: a content line starting with "++" is not a file header, and nothing is printed in clear.
func TestScanDiffIgnoresHeaderLookalikes(t *testing.T) {
	tok := fake("gh"+"p_", "Zx8Qw3Er5Ty7Ui9Op1As2Df4Gh6Jk8Lz0Xc2V")
	diff := "diff --git a/c.txt b/c.txt\n--- a/c.txt\n+++ b/c.txt\n@@ -0,0 +1,3 @@\n+x=1\n+++ " + tok + "\n+" + fake("AK"+"IA", "Z7Q2X4M8N3P5R6TW") + "\n"
	fs := ScanDiff(diff)
	if len(fs) != 2 {
		t.Fatalf("both secrets must be found: %+v", fs)
	}
	for _, f := range fs {
		if f.File != "c.txt" || strings.Contains(f.File+f.Match, tok) {
			t.Fatalf("wrong file or secret in clear: %+v", f)
		}
	}
	if fs[1].Line != 3 {
		t.Fatalf("line numbers must count every added line: %+v", fs)
	}
}

func TestScanDiffLineNumbers(t *testing.T) {
	diff := "diff --git a/x.env b/x.env\n+++ b/x.env\n@@ -0,0 +10,2 @@\n+OK=1\n+GH=" + fake("gh"+"p_", "aB3dE5fG7hI9jK1lM3nO5pQ7rS9tU1vW3xY5") + "\n"
	fs := ScanDiff(diff)
	if len(fs) != 1 || fs[0].File != "x.env" || fs[0].Line != 11 {
		t.Fatalf("got %+v", fs)
	}
}
