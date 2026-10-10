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

func TestOnlyJSONRPCMessagesPass(t *testing.T) {
	for _, bad := range []string{`123`, `null`, `"x"`, `[]`, `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, `{}`, `{"jsonrpc":"1.0","id":1,"method":"ping"}`, `{"jsonrpc":"2.0","id":{},"method":"ping"}`,
		`{"jsonrpc":"2.0","result":{}}`, `{"jsonrpc":"2.0","id":null,"result":{}}`, `{"jsonrpc":"2.0","id":9,"error":"x"}`,
		`{"jsonrpc":"2.0","id":9,"error":{"code":"x"}}`, `{"jsonrpc":"2.0","id":"a","result":{}}`, `{"jsonrpc":"2.0","id":1.5,"method":"ping"}`} {
		if isMessage([]byte(bad)) {
			t.Errorf("%s must be rejected", bad)
		}
	}
	for _, good := range []string{`{"jsonrpc":"2.0","id":1,"method":"ping"}`, `{"jsonrpc":"2.0","method":"notifications/initialized"}`} {
		if !isMessage([]byte(good)) {
			t.Errorf("%s must pass", good)
		}
	}
}
