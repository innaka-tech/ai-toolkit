# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [2.4.0] - 2026-10-10

Autonomous work with the same Definition of Done.

### Added
- **`aitk run`**: works through ready tasks with a headless agent (`--agent claude-code|codex|opencode|gemini-cli`, or `--cmd` for any tool), one task at a time. A task counts as finished only when aitk re-derives it: fields an agent must not change for itself (user acceptance, review passes, audit, regression evidence, profile and pin, tags, goal, dependencies; criteria may be added and checked, not removed or reworded) are restored to their values before the attempt, aitk runs the check (and a required audit) itself, recomputes the regression evidence, and applies the Definition of Done; anything else is reopened. Tasks needing a person stay `in_review`; unfinished tasks are handed over as `blocked` with a handoff after `--max-attempts`; the run stops after two unfinished tasks in a row or when the agent cannot start. A timeout, and the end of each attempt, ends the agent's process group. One run per worktree (kernel lock); nested runs are refused; agents it starts cannot accept UAT. `--commit` (clean tree required) makes one Conventional Commit per finished task; for unfinished work it commits only aitk's handover record and stashes the rest, so no commit mixes tasks. The prompt is passed as a file. `--dry-run` shows the order. Configurable in `[run]`.
- **`aitk task next`** (and MCP `task_next`): the tasks that can start now, in order, honouring `depends_on` and other worktrees' claims; `--start` takes the first. `aitk task start` warns about unfinished dependencies.
- **`aitk impact`** (and MCP `impact`): for each changed source file, the code that references it (by name, and by Go import path) and the tests that cover it; warns about changed files no test covers and about deleted files that code still references.
- **`aitk audit --tasks`**: findings become fix tasks, one per vulnerable package (osv-scanner JSON; every installed version with the version that fixes it) and one per other failing scanner, without duplicating open ones. Version comparison understands SemVer, PEP 440, and Maven qualifiers.
- **Regression tests for bug fixes**: a task tagged `bug` cannot be done until a test file changed with it: in the commits made since the task was created (except other tasks' commits, by their `AI-Task` trailer) or in uncommitted changes; deleting a test does not count (`E_DOD_REGRESSION_TEST`; `[quality] regression_tests = false` turns it off). The test files are recorded as evidence.
- **Spec Kit, both ways**: phases, `[P]` markers, and explicit "(depends on T012, T013)" become `depends_on`, `[US1]` markers become tags, tasks link the feature's spec.md and plan.md; closing a task checks off its line in tasks.md (also OpenSpec and markdown imports); importing again re-checks done tasks and reports lines checked only in the file.

### Changed
- The AGENTS.md block and the Agent Skill mention `aitk task next` and `aitk impact`; `aitk doctor --fix` updates the block. The block now carries `aitk:block-revision`: the pre-commit gate accepts earlier and later releases' blocks (and CRLF checkouts) and rejects only a hand-edited one, and aitk never replaces a newer block with its own, so teammates on 2.4 and later can mix versions. Teammates on 2.3 or older should upgrade: their gate rejects the 2.4 block, and they reject the new `[run]` and `[quality]` settings.

## [2.3.0] - 2026-10-09

### Added
- `aitk audit install` installs osv-scanner, the dependency vulnerability scanner `aitk audit` uses for every ecosystem: Homebrew on macOS, otherwise the official release binary after its SHA-256 is verified against the release checksum file (`--dir`, `--no-brew`).
- `install.sh` now installs osv-scanner along with aitk (skip with `AITK_NO_SCANNER=1`), so audits work out of the box.
- `aitk doctor` warns when a project has dependency manifests but no scanner is installed, with the fix command; `aitk audit` warnings name it too.

### Fixed
- `aitk audit` failed in repositories without dependency manifests once osv-scanner was installed (osv-scanner exits 128 when there is nothing to scan). With manifests present, exit 128 still fails the audit.

## [2.2.0] - 2026-10-09

Quality and delivery standards for the projects aitk manages.

### Added
- **User acceptance (UAT)**: `uat.required` per profile; `aitk uat script|accept|reject`; only a person can accept (agents are refused); rejection reopens the task and reaches the next agent through the brief.
- **Security standards**: OWASP ASVS 4.0.3 checklist required for strict tasks (`aitk security checklist`); `aitk audit` runs dependency scanners (osv-scanner, npm/pnpm audit, govulncheck, pip-audit, composer audit, cargo audit), semgrep OWASP Top 10, and a secret scan of tracked files, recorded as evidence and required by the Definition of Done (`security.audit`).
- **Project versioning**: `aitk release` computes the next SemVer from Conventional Commits, writes a Keep a Changelog section (with commit hashes and task ids), updates `package.json` / `pyproject.toml` / `Cargo.toml` and `ai-state.json`, and with `--tag` commits and tags; pre-releases; refuses dirty trees and failing checks; never pushes.
- **Conventions**: `docs/ai/conventions.md` created by init and shown in every brief; doctor flags missing or unfilled conventions.
- **Goals**: `aitk goal add|list|status`; tasks link with `--goal`; brief and `docs/ai/goals.md` show progress.
- **Integrations**: `aitk import bmad` (BMAD Method tickets and plan status), `aitk import superpowers` (Superpowers plans), `aitk import markdown`; Agent Skill `skills/aitk/SKILL.md`, installed for Claude Code by `adapters sync`; MCP tools `audit`, `security_checklist`, `uat_script`.

### Fixed
- Handoffs written in the same second were ordered wrongly, so a UAT rejection could be missing from the brief.
- Relative paths given to `aitk import` resolved against the process directory instead of aitk's working directory.

## [2.1.1] - 2026-10-09

### Fixed
- The brief hid most project knowledge whenever files were changed (only entries matching the changed paths were kept). It now orders all knowledge by relevance and fills the remaining slots with pinned and recent entries; `knowledge search` still returns matches only.

## [2.1.0] - 2026-10-09

### Added
- **Self-control** (spec §9): consecutive failing checks tell agents to stop and ask or hand over; credentials in recorded text are refused (`E_SECRET`); large changes are flagged; `task update` cannot mark work done.
- **Self-healing** (spec §10): damaged task frontmatter is normalized, unparseable files are restored from git, merge conflicts in aitk files are resolved, drifted adapters and a missing check command are repaired, before every writing command and by `aitk doctor --fix`. Nothing is discarded; leftovers go to `docs/ai/_legacy/quarantine/`.

### Fixed (from an independent review; each has a regression test)
- Knowledge compaction and pinning could delete hand-written prose, headings, comments, and the second paragraph of an entry; they now edit entry blocks only.
- `init` and later commands could overwrite hand-written `handoff.md`, `knowledge.md`, `decisions.md`, `current-task.md`.
- Ticking criteria rewrote the whole section; nested items were counted as criteria.
- `hooks install` broke hooks written in other languages; they are now wrapped.
- Adapter and hook writes replaced symlinks and widened permissions (a 0600 config became world-readable).
- Stale checks passed for non-ASCII file names and in repositories without commits.
- A pinned `lite` profile overrode sensitive paths; strict review independence ignored who implemented the task; `task update --status done` skipped the Definition of Done.
- MCP free text starting with `-` was parsed as flags.
- JSON configs with trailing content were silently truncated.

## [2.0.2] - 2026-10-09

Findings from the first real-project pilot.

### Fixed
- `aitk migrate` no longer turns the v1 "no active task" placeholder (`Task ID: none`, `Status: IDLE`) into a task.
- aitk's own metadata (`aitk.toml`, `AGENTS.md`, `CLAUDE.md`, `.gitattributes`, `docs/adr/`, `.aitk/`) no longer counts toward the risk profile or makes a passing check stale; right after a migration the brief listed them as "changed files" and raised the profile.
- `aitk version` shows the module version when installed with `go install`.

## [2.0.1] - 2026-10-09

### Fixed
- `go install` failed for v2: the module path is now `github.com/innaka-tech/ai-toolkit/v2` (Go requires the major-version suffix). Install with `go install github.com/innaka-tech/ai-toolkit/v2/cmd/aitk@latest`.
- The GitHub Action installs the latest v2 release by default instead of `main`.

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
