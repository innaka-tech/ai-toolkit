package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func osvServer(t *testing.T, body []byte, sum string, asset string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/osv-scanner_SHA256SUMS":
			io.WriteString(w, sum+"  "+asset+"\n"+strings.Repeat("0", 64)+"  osv-scanner_plan9_mips\n")
		case "/" + asset:
			w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := OSVReleaseURL
	OSVReleaseURL = srv.URL
	t.Cleanup(func() { OSVReleaseURL = old })
	t.Setenv("PATH", t.TempDir()) // no osv-scanner installed
}

func asset() string {
	a := "osv-scanner_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		a += ".exe"
	}
	return a
}

func TestInstallOSVVerifiesChecksum(t *testing.T) {
	body := []byte("fake osv-scanner binary")
	h := sha256.Sum256(body)
	osvServer(t, body, hex.EncodeToString(h[:]), asset())
	dir := filepath.Join(t.TempDir(), "bin")
	r, err := InstallOSV(dir, false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "download" || filepath.Dir(r.Path) != dir {
		t.Fatalf("unexpected result: %+v", r)
	}
	if b, _ := os.ReadFile(r.Path); string(b) != string(body) {
		t.Fatal("installed binary differs from the download")
	}
}

func TestInstallOSVRejectsChecksumMismatch(t *testing.T) {
	osvServer(t, []byte("tampered"), strings.Repeat("a", 64), asset())
	dir := filepath.Join(t.TempDir(), "bin")
	if _, err := InstallOSV(dir, false, io.Discard); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("nothing may be installed after a mismatch, found %d files", len(entries))
	}
}

func TestInstallOSVUnsupportedPlatform(t *testing.T) {
	osvServer(t, nil, strings.Repeat("a", 64), "osv-scanner_other_arch")
	if _, err := InstallOSV(t.TempDir(), false, io.Discard); err == nil || !strings.Contains(err.Error(), "no osv-scanner release") {
		t.Fatalf("want unsupported-platform error, got %v", err)
	}
}

func TestHasManifest(t *testing.T) {
	dir := t.TempDir()
	if HasManifest(dir) {
		t.Fatal("empty dir has no manifest")
	}
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644)
	if !HasManifest(dir) {
		t.Fatal("go.mod is a manifest")
	}
}
