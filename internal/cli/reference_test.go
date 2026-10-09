package cli

import (
	"os"
	"strings"
	"testing"
)

func TestReferenceIsUpToDate(t *testing.T) {
	want := Reference()
	got, err := os.ReadFile("../../docs/reference/cli.md")
	// Windows checkouts may convert line endings to CRLF.
	if err != nil || strings.ReplaceAll(string(got), "\r\n", "\n") != want {
		t.Fatal("docs/reference/cli.md is stale; run: go run ./cmd/aitk __reference > docs/reference/cli.md")
	}
}
