# Contributing

Thanks for helping. aitk aims to stay small, reliable, and boring to operate.

## Before you start

- Read the [specification](docs/spec/README.md) and the [ADRs](docs/adr/README.md). Behavior changes start as a spec change (and an ADR when they change a decision).
- Open an issue for anything larger than a bug fix so the design can be agreed first.

## Development

```bash
go build ./cmd/aitk
go vet ./... && go test -race ./...
go run ./cmd/aitk __reference > docs/reference/cli.md   # after changing commands or flags
bash tests/smoke.sh                                    # v1 regression
AITK=$(pwd)/aitk bash tests/shims.sh                   # ai-* compatibility shims
```

Requirements: Go (see `go.mod`), git. CI runs on Linux, macOS, and Windows; keep code portable (no shell-isms in Go, paths through `filepath`).

## Rules of the road

- **Tests with behavior.** Every change in behavior comes with a test that fails without it. End-to-end tests drive the real CLI against temporary git repositories.
- **Never lose user data.** Writes are atomic and locked; migration and compaction are lossless; adapters and hooks edit only what aitk owns.
- **Errors tell the fix.** Every error has a stable `E_*` code, an exit code from the spec, and a `fix` hint.
- **No secrets in the repository**, including tests: build token-shaped test data at runtime.
- Commits follow [Conventional Commits](https://www.conventionalcommits.org); pull requests keep CI green.

## Releases

Maintainers tag `vX.Y.Z` after updating `CHANGELOG.md`; the release workflow builds, checksums, signs (cosign keyless), and publishes. Versions follow [SemVer](https://semver.org).
