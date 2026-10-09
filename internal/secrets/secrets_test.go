package secrets

import "testing"

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
		"anthropic-api-key":         "ANTHROPIC_API_KEY=" + fake("sk-"+"ant-", "api03-Zq8xW2vY4uT6sR8pN0mL2kJ4hG6fD8sA0qW"),
		"cloudflare-api-token":      "Authorization: Bearer " + fake("cf"+"at_", "Qx7Lm2Np4Rs6Tv8Wx0Yz1Ab3Cd5EfGh7Jk9Mn2Pq4"),
		"jwt":                       fake("ey"+"J", "hbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"),
		"generic-secret-assignment": `DB_PASSWORD="k8#Vq2!xR9zL4mPw"`,
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

func TestScanDiffLineNumbers(t *testing.T) {
	diff := "diff --git a/x.env b/x.env\n+++ b/x.env\n@@ -0,0 +10,2 @@\n+OK=1\n+GH=" + fake("gh"+"p_", "aB3dE5fG7hI9jK1lM3nO5pQ7rS9tU1vW3xY5") + "\n"
	fs := ScanDiff(diff)
	if len(fs) != 1 || fs[0].File != "x.env" || fs[0].Line != 11 {
		t.Fatalf("got %+v", fs)
	}
}
