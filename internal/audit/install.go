package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// OSVReleaseURL is where osv-scanner release assets are downloaded from.
var OSVReleaseURL = "https://github.com/google/osv-scanner/releases/latest/download"

// Install is the result of InstallOSV.
type Install struct {
	Path    string `json:"path"`
	Method  string `json:"method"` // present | brew | download
	Version string `json:"version,omitempty"`
}

// DefaultBinDir is where downloaded scanners go: next to aitk's own default install.
func DefaultBinDir() string {
	if runtime.GOOS == "windows" {
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "aitk", "bin")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

// InstallOSV makes osv-scanner available: it keeps an installed one, uses Homebrew on macOS
// when present, and otherwise downloads the official release binary into dir after verifying
// its SHA-256 against the release's checksum file.
func InstallOSV(dir string, useBrew bool, log io.Writer) (*Install, error) {
	if p, err := exec.LookPath("osv-scanner"); err == nil {
		return &Install{Path: p, Method: "present", Version: osvVersion(p)}, nil
	}
	if useBrew && runtime.GOOS == "darwin" {
		if brew, err := exec.LookPath("brew"); err == nil {
			c := exec.Command(brew, "install", "osv-scanner")
			c.Stdout, c.Stderr = log, log
			if err := c.Run(); err != nil {
				return nil, fmt.Errorf("brew install osv-scanner: %w", err)
			}
			p, err := exec.LookPath("osv-scanner")
			if err != nil {
				return nil, fmt.Errorf("brew installed osv-scanner but it is not on PATH")
			}
			return &Install{Path: p, Method: "brew", Version: osvVersion(p)}, nil
		}
	}
	asset := "osv-scanner_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		asset += ".exe"
	}
	sums, err := fetch(OSVReleaseURL + "/osv-scanner_SHA256SUMS")
	if err != nil {
		return nil, err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			want = strings.ToLower(f[0])
		}
	}
	if want == "" {
		return nil, fmt.Errorf("no osv-scanner release for %s/%s (%s is not in the checksum file)", runtime.GOOS, runtime.GOARCH, asset)
	}
	fmt.Fprintf(log, "downloading %s\n", asset)
	bin, err := fetch(OSVReleaseURL + "/" + asset)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(bin)
	if got := hex.EncodeToString(sum[:]); got != want {
		return nil, fmt.Errorf("checksum mismatch for %s (expected %s, got %s); nothing was installed", asset, want, got)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := "osv-scanner"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dest := filepath.Join(dir, name)
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	return &Install{Path: dest, Method: "download", Version: osvVersion(dest)}, nil
}

func fetch(url string) ([]byte, error) {
	c := &http.Client{Timeout: 5 * time.Minute}
	resp, err := c.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 512<<20))
}

func osvVersion(path string) string {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(strings.TrimPrefix(line, "osv-scanner version:"))
}

// HasManifest reports whether the project at root has dependency manifests that a
// vulnerability scanner can check.
func HasManifest(root string) bool {
	for _, f := range []string{"package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.mod", "requirements.txt", "pyproject.toml", "poetry.lock", "composer.lock", "Cargo.lock", "Gemfile.lock", "pom.xml", "build.gradle"} {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			return true
		}
	}
	return false
}
