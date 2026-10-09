// Package plugins discovers and calls aitk-<name> executables (ADR-0007, docs/spec/integrations.md).
package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/innaka-tech/ai-toolkit/internal/project"
	"github.com/innaka-tech/ai-toolkit/internal/schema"
)

const maxOutput = 1 << 20 // 1 MiB

// Plugin is a discovered executable.
type Plugin struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Enabled bool     `json:"enabled"`
	Version string   `json:"version,omitempty"`
	Hooks   []string `json:"hooks,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// Discover lists aitk-* executables on PATH (first match wins, like the shell).
func Discover(cfg project.Config) []Plugin {
	enabled := map[string]bool{}
	for _, n := range cfg.Plugins.Enabled {
		enabled[n] = true
	}
	seen := map[string]bool{}
	var out []Plugin
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if runtime.GOOS == "windows" {
				if !strings.HasSuffix(strings.ToLower(name), ".exe") {
					continue
				}
				name = name[:len(name)-4]
			}
			if !strings.HasPrefix(name, "aitk-") || e.IsDir() {
				continue
			}
			n := strings.TrimPrefix(name, "aitk-")
			if n == "" || seen[n] {
				continue
			}
			full := filepath.Join(dir, e.Name())
			if info, err := os.Stat(full); err != nil || (runtime.GOOS != "windows" && info.Mode()&0o111 == 0) {
				continue
			}
			seen[n] = true
			out = append(out, Plugin{Name: n, Path: full, Enabled: enabled[n]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func timeout(cfg project.Config) time.Duration {
	if d, err := time.ParseDuration(cfg.Plugins.Timeout); err == nil && d > 0 {
		return d
	}
	return 5 * time.Second
}

func run(ctx context.Context, path string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var out limited
	out.max = maxOutput
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("timed out")
	}
	if out.over {
		return nil, fmt.Errorf("output exceeds 1 MiB")
	}
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type limited struct {
	bytes.Buffer
	max  int
	over bool
}

func (l *limited) Write(p []byte) (int, error) {
	if l.Len()+len(p) > l.max {
		l.over = true
		return 0, errors.New("output limit")
	}
	return l.Buffer.Write(p)
}

// Manifest asks a plugin for its manifest.
func Manifest(cfg project.Config, p *Plugin) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout(cfg))
	defer cancel()
	b, err := run(ctx, p.Path, nil, "manifest")
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("manifest is not JSON: %v", err)
	}
	if err := schema.Validate("plugin", m); err != nil || m["schema"] != "aitk.plugin.manifest/v1" {
		return fmt.Errorf("invalid manifest: %v", err)
	}
	p.Version, _ = m["version"].(string)
	for _, h := range m["hooks"].([]any) {
		p.Hooks = append(p.Hooks, h.(string))
	}
	return nil
}

// Result of one hook call.
type Result struct {
	Plugin string          `json:"plugin"`
	Data   json.RawMessage `json:"data,omitempty"`
	Err    string          `json:"error,omitempty"`
}

// Call invokes hook on every enabled plugin that declares it. Failures become Result.Err.
func Call(p *project.Project, hook string, payload map[string]any, task any) []Result {
	var out []Result
	for _, pl := range Discover(p.Config) {
		if !pl.Enabled {
			continue
		}
		pl := pl
		if err := Manifest(p.Config, &pl); err != nil {
			out = append(out, Result{Plugin: pl.Name, Err: err.Error()})
			continue
		}
		declared := false
		for _, h := range pl.Hooks {
			declared = declared || h == hook
		}
		if !declared {
			continue
		}
		if payload == nil {
			payload = map[string]any{}
		}
		if s, ok := p.Config.Plugins.Settings[pl.Name]; ok {
			payload["settings"] = s
		}
		req := map[string]any{"schema": "aitk.plugin.request/v1", "hook": hook,
			"project": map[string]any{"root": p.Root, "name": p.Config.Project.Name}, "task": task, "payload": payload}
		body, _ := json.Marshal(req)
		ctx, cancel := context.WithTimeout(context.Background(), timeout(p.Config))
		b, err := run(ctx, pl.Path, body, "hook", hook)
		cancel()
		if err != nil {
			out = append(out, Result{Plugin: pl.Name, Err: err.Error()})
			continue
		}
		var resp map[string]any
		if err := json.Unmarshal(b, &resp); err != nil || schema.Validate("plugin", resp) != nil || resp["schema"] != "aitk.plugin.response/v1" {
			out = append(out, Result{Plugin: pl.Name, Err: "invalid response"})
			continue
		}
		if ok, _ := resp["ok"].(bool); !ok {
			msg, _ := resp["error"].(string)
			out = append(out, Result{Plugin: pl.Name, Err: "plugin error: " + msg})
			continue
		}
		data, _ := json.Marshal(resp["data"])
		out = append(out, Result{Plugin: pl.Name, Data: data})
	}
	return out
}
