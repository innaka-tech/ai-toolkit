# Conventions

<!-- Every agent sees the first lines of this file in its brief. Keep it short and specific. -->

- Stack and versions: Go (see go.mod), single static binary `cmd/aitk`; cobra CLI, BurntSushi/toml, go.yaml.in/yaml/v3, santhosh-tekuri/jsonschema v6, modelcontextprotocol/go-sdk
- Structure: commands in `internal/cli`, logic in `internal/ops` and focused packages under `internal/`; JSON Schemas in `schemas/` (embedded); spec in `docs/spec/`, ADRs in `docs/adr/`
- Naming and style: `gofmt`, `go vet`; match the surrounding code; comments explain why, not what
- Errors: user-facing failures are `apperr.New(code, exit, fix, msg)` with an `E_*` code listed in docs/spec/cli.md and a runnable `fix:`; exit codes 0–4 only
- Data: aitk files are plain text in git; every write is atomic (`fsx`) and validated against its schema; never print or store secrets
- Tests: `go test ./...` (CI adds -race on Linux, macOS, Windows); every bug fix gets a regression test; CLI behaviour is tested through `aitk(t, dir, …)` in internal/cli or the built binary in internal/gates
- Security: no network except `aitk audit install` and plugins the user enables; downloads are checksum-verified; agents cannot accept UAT or set done
- Git and versioning: Conventional Commits, SemVer, Keep a Changelog; `docs/reference/cli.md` regenerated with `go run ./cmd/aitk __reference`
- UI consistency: CLI output has a human form and `--json` (aitk.result/v1 envelope); keep human output terse
