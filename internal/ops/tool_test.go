package ops

import "testing"

func TestToolDetection(t *testing.T) {
	vars := []string{"AITK_TOOL", "CLAUDECODE", "CODEX_SESSION_ID", "CODEX_VERSION", "CODEX_SANDBOX", "CODEX_MANAGED_BY_NPM", "GEMINI_CLI", "OPENCODE"}
	cases := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"CLAUDECODE": "1"}, "claude-code"},
		{map[string]string{"CLAUDECODE": "1", "CODEX_SESSION_ID": "x"}, "codex"}, // Codex started from Claude Code
		{map[string]string{"GEMINI_CLI": "1", "CLAUDECODE": "1"}, "gemini-cli"},
		{map[string]string{"OPENCODE": "1"}, "opencode"},
		{map[string]string{"CLAUDECODE": "1", "AITK_TOOL": "My Bot"}, "my-bot"},
	}
	for _, c := range cases {
		for _, v := range vars {
			t.Setenv(v, "")
		}
		for k, v := range c.env {
			t.Setenv(k, v)
		}
		if got := Tool(); got != c.want {
			t.Errorf("env %v: got %q, want %q", c.env, got, c.want)
		}
	}
}
