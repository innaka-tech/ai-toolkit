# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [2.0.0] - 2026-10-09

First release of `aitk`, a rewrite of the toolkit as a single Go binary. See the [README](README.md) and [specification](docs/spec/README.md).

### Added
- `aitk` v2 core in Go (single binary): `init`, `migrate`, `brief`, `task new|start|list|show|update|block`, `check`, `close`, `review pass`, `knowledge add|search|compact|pin`, `adr new|list`, `doctor`, `version`; `--json` on every command (`aitk.result/v1`), exit codes 0–4, error codes with fix hints.
- Definition of Done per risk profile (lite/standard/strict), stale-check detection by working-tree fingerprint, independent review passes for strict tasks.
- Lossless, idempotent v1 → v2 migration; validated on 24 real v1 projects (0 doctor errors, every brief ≤ 4k tokens, ~100 ms).
- Go CI on Linux, macOS, Windows with `-race`; govulncheck.

- `aitk mcp`: MCP server (official Go SDK) exposing 13 tools, the `aitk://brief` resource, and `start-session`/`close-session` prompts; every tool runs the CLI code path; the MCP client name becomes the recorded tool. Verified end-to-end with Claude Code and OpenCode.
- `aitk adapters sync|doctor` for Claude Code, Codex, OpenCode, Gemini CLI, Kiro, and Cursor: MCP registration, instructions, and `SessionStart` → `aitk brief` hooks; order-preserving JSON edits, idempotent, global edits backed up.

- Gates: `aitk hooks install|uninstall` (pre-commit secret scan + schema validation, Conventional Commits with `AI-Task`/`AI-Tool` trailers, pre-push check; existing hooks and `core.hooksPath` respected), `aitk ci`, and a composite GitHub Action.

- Parallel work: task claims (`task claim|release`, automatic on start/close), `aitk work` (worktree + branch per task), union-merge `.gitattributes` so parallel branches merge without conflicts (tested: 3 worktrees, 0 conflicts).
- `aitk switch <tool>` (handoff + brief prompt; starts Claude Code, Codex, Gemini CLI, OpenCode), `aitk report`, `aitk log`.
- Plugins: `aitk-<name>` executables with JSON hooks `brief.sections`, `close.after`, `doctor.checks`, `knowledge.search`; per-plugin settings; timeouts and failures are warnings; `aitk plugin list`.
- `aitk import github-issues|spec-kit|openspec`, `aitk deploy`.

### Changed
- Project lock is now a kernel lock (flock / LockFileEx); no stale locks.
- Agent sandboxes: when `.git` is read-only (Codex `workspace-write`), private state falls back to a self-ignoring `.aitk/` and reads merge both locations; lock errors distinguish contention (`E_LOCKED`) from unwritable state (`E_STATE_UNWRITABLE`).
- Tool attribution checks the innermost tool first (Codex/Gemini/OpenCode before Claude Code) when tools run inside each other.
- `AI-Task` commit trailer also names a task closed in the last hour.
- Definition of Done: criteria written on a task must be satisfied under every profile; the profile comes from the diff unless pinned.
- Spec: config is validated from TOML; knowledge entries may carry `file`; checks record a `tree` fingerprint; session keeps `last_check`; hand-written and v1-frontmatter task files migrate (ID collisions get a fresh ID with `legacy.v1_id`).

### Validation
- 1,000 random operations (property test in CI): no corruption, no lost knowledge, zero doctor errors.
- Live agents, given only the coding task: Claude Code (Haiku) created, checked, and closed its task and committed with trailers; Codex continued a task handed over by Claude Code from the repository state alone and closed it.

### Notes
- English only; Indonesian output is planned for 2.1 (ADR-0010).
- `ai-*` commands are now shims: they forward to `aitk` in v2 projects and run the frozen v1 scripts (`scripts/v1/`) in v1 projects.
- `install.sh` installs the v2 binary; the v1 installer is `scripts/v1/install-v1.sh`.

### Removed
- Python migration prototype (replaced by `aitk migrate`).

## [1.0.0] - 2026-10-09

Final release of the bash implementation. v2 (`aitk`) will be a rewrite; `ai-*` commands will keep working as shims.

### Removed
- Private agent-coordination layer (`ai-agent-report`, `ai-agent-report-jaka`, `ai-handoff`, `ai-handoff-remote`, `FUSION_AI_PROTOCOL.md`, bridge queue). It now lives in a separate private repository.
- Account rotation (`ai-rotate-ag`) and placeholder provider/cost settings in `config.yaml`.
- Keyword-based provider auto-selection in `ai-exec`.

### Fixed
- `ai-loop` sent an empty prompt to every agent: the prompt variable was not visible inside `bash -c`. Agents now receive the prompt as a single argument.
- `ai-loop` never ran the agent when neither `timeout` nor `gtimeout` was installed. A portable perl fallback now enforces the timeout (exit 124).
- `ai-exec --auto` / `--agent NAME` (as documented) are now accepted; unknown providers exit 2 instead of 0.
- Duplicate `mcp` entry in `ai-toolkit` help and dispatcher.
- Absolute personal path in README.

### Added
- `tests/smoke.sh`: end-to-end check of init → start → preflight → exec → close in an isolated HOME.
- CI: ShellCheck, syntax check, smoke test on Linux and macOS, gitleaks.
