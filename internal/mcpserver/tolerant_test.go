package mcpserver

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// Bug hunt: a malformed line gets a parse error; the session goes on.
func TestTolerantTransport(t *testing.T) {
	in := strings.NewReader("{not json\n\n{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n")
	var replies bytes.Buffer
	r := tolerant(in, &replies)
	got, _ := io.ReadAll(r)
	if string(got) != "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n" {
		t.Fatalf("valid lines must pass through unchanged: %q", got)
	}
	if !strings.Contains(replies.String(), "-32700") {
		t.Fatalf("a malformed line must get a parse error: %q", replies.String())
	}
}
