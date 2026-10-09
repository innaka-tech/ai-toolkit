package cli

import (
	"os"
	"testing"
)

func TestReferenceIsUpToDate(t *testing.T) {
	want := Reference()
	got, err := os.ReadFile("../../docs/reference/cli.md")
	if err != nil || string(got) != want {
		t.Fatal("docs/reference/cli.md is stale; run: go run ./cmd/aitk __reference > docs/reference/cli.md")
	}
}
