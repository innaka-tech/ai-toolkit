# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [2.6.0] - 2026-10-10

Slash commands and personas in every AI tool.

### Added
- **13 built-in slash commands**: `/aitk-start`, `/aitk-plan`, `/aitk-next`, `/aitk-check`, `/aitk-impact`, `/aitk-close`, `/aitk-handover`, `/aitk-status`, and the personas `/aitk-review` (independent reviewer that records `aitk review pass` and never starts the task), `/aitk-hunt` (bug hunter that files each proven defect as a `bug` task), `/aitk-fix` (test-first fixer), `/aitk-uat` (acceptance script writer), and `/aitk-release` (release manager).
- **Native in each tool**: Claude Code, Codex (custom prompts), OpenCode, Gemini CLI and Qwen Code (TOML with `{{args}}`), Cursor, GitHub Copilot prompt files (`${input:args}`), Windsurf workflows. Built-ins are installed at user level by `aitk setup` where a tool has a user folder, otherwise in the repository by `aitk adapters sync`.
- **Project commands**: a markdown file in `docs/ai/commands/` becomes a slash command in every tool, and an MCP prompt; it replaces a built-in of the same name. Generated files are marked, kept current, and removed when their source is gone. A user's own command with the same name is never overwritten. `aitk init` adds a README with an example.
- **MCP prompts**: every command, with an `args` argument (`start-session` and `close-session` remain as aliases).
- **`aitk prompt [name] [text…]`**: lists the commands, or prints one with the text filled in, for any tool or a pipe (`aitk prompt aitk-hunt payments | claude -p`).
- **`aitk task new --depends <id>`** (and `depends_on` in MCP `task_new`), used by `/aitk-plan`.

## [2.5.1] - 2026-10-10

Fixes from an independent bug hunt across the whole tool (28 reproduced defects).

### Fixed
- **Risk profile**:
  - Work a task already committed on the default branch counts toward its profile. Tasks record the commit they started from (`base`).
  - Moving a file out of a sensitive path makes the task strict.
  - Before, both could close as `lite` without the strict gates.
- **Check freshness**:
  - The fingerprint is now the content of the code (a git tree), so committing the checked code no longer makes the check stale.
  - Re-pointed symlinks and moved submodules do.
  - Existing check evidence becomes stale once after upgrading: run `aitk check` again.
- **Tasks**:
  - `close` refuses cancelled and done tasks.
  - Cancelling or blocking a task clears it as the active task and releases its claim.
  - A missing active task points to `aitk doctor --fix` and is named in the brief.
  - `uat reject` only applies to work waiting for acceptance.
  - `aitk work <id>` run again reuses the task's worktree.
- **Self-healing**: a Markdown setext heading (`=======`) is no longer treated as a merge conflict. Before, it was deleted, or the file was quarantined on every command.
- **Knowledge**: findings that differ only in a number's punctuation ("1.5s" and "15s") are no longer merged as duplicates.
- **Secret scan**:
  - An added line starting with `++` can no longer pose as a file header, which skipped detection and printed a secret in clear.
  - Newly detected: unquoted `.env` values, keys such as `SECRET_KEY` and `PASSWORD_PROD`, credentials in URLs, and SSH2 private keys.
  - An Anthropic key is reported once.
- **Commit hook**: an empty message aborts the commit again; before, a trailer became the subject. `git commit -v` and `Reapply "…"` messages are handled.
- **Release**:
  - Only `[package]` (Cargo), `[project]`, or `[tool.poetry]` versions are changed. Before, a dependency's version could be rewritten.
  - package.json is edited in place.
  - A non-SemVer tag (`v2-beta`, `+build` metadata) no longer resets the version to the initial one.
  - `Revert "…"` commits are listed.
  - A first release with only chores works.
- **Timeouts**: `aitk check`, plugins, and `aitk run` end the whole process group on timeout and after finishing. A background process no longer hangs aitk or the MCP server.
- **Imports**:
  - Superpowers and BMAD plans with long names keep every task (IDs were truncated into one).
  - Markdown and OpenSpec items are matched by text, so inserting an item no longer loses another.
  - Checkboxes in code blocks and nested sub-steps are not tasks.
  - GitHub issue criteria keep their checked state; CRLF is handled.
- **Migration**:
  - `project.env` values with `export`, single quotes, or trailing comments are read correctly.
  - A v1 task in both `current-task.md` and a `.previous.md` snapshot becomes one task.
  - `*`, `+`, and numbered items and paragraphs in knowledge.md become entries.
- **MCP**:
  - A malformed JSON line gets a JSON-RPC parse error instead of ending the server.
  - The `aitk://task/{id}` resource exists, as documented.
- **CLI**:
  - Group commands (`aitk task`) and unknown subcommands answer with an envelope under `--json`, and with E_USAGE for typos.
  - `--json` after `--` is an argument.
  - `review pass --findings -3` says why it is refused.
- **Adapters**: JSON config files with a UTF-8 byte order mark are edited.
- **Default branch detection**: works with packed and reftable refs.
- **v1 shims**: `ai-init <dir>`, `ai-commit "<msg>"`, and `ai-push` work in v2 projects.
- **Spec**: documents only what exists (no `--no-color`, `--yes`, `AITK_LANG`, or user config file).
- **Second verification round (by the same hunters)**:
  - **Risk profile:**
    - A task's work stays in its profile after `git commit --amend` or a rebase.
    - A close without a task covers what was committed since the last close.
  - **Fingerprint:**
    - Objects go to a throwaway directory, so a check writes nothing into `.git` and works when `.git` is read-only.
    - Either fingerprint kind is accepted at close.
    - Uncommitted work inside submodules counts.
  - **Secret scan:** unquoted values are flagged only on `.env`-style `UPPER_KEY=value` lines, and identifiers, URLs, paths, URNs, and test database URLs are not secrets. On 1,149 files of real Go code, findings went from 135 to 0.
  - **MCP:** JSON that is not a JSON-RPC 2.0 message (`123`, `[]`, batches, `{}`) gets -32600 without ending the session. Lines over 32 MiB are skipped with an error. Requests read just before stdin closes are still answered.
  - **Imports:**
    - Items with the same text are told apart by order.
    - Nested sub-steps become the parent task's criteria, with their checked state.
    - Closing a task checks off the right occurrence.
    - Import paths are resolved through symlinks (macOS `/var`), which had broken write-back.
  - **Commands and help:**
    - A check whose detached child keeps the output open reports the check's own exit status.
    - `--json --help` prints an envelope.
    - Doctor's fix line no longer suggests `--fix` after it already ran.
  - **Release:** `[workspace.package]` versions are bumped, and reverts are labelled "Revert" in the changelog.
  - **v1 shims:** `ai-commit` keeps `--push` and `--deploy` as options, and `ai-push [path]` sets the upstream on the first push.
- **Third verification round**:
  - **Risk profile:** the first close without a task counts everything since the commit that added `aitk.toml`.
  - **MCP:**
    - Only requests and notifications are accepted from the client. Stray responses and fractional ids get -32600 instead of ending the session.
    - At end of input, the server waits until every request read has been answered, including a slow `check`.
  - **Fingerprint:** untracked files over 32 MiB count by size and modification time instead of being copied on every check.
  - **Secret scan:** newly covered are docker-compose `KEY: value` and `- KEY=value`, Dockerfile `ENV`, `docker run -e`, `aws_secret_access_key`, `.npmrc` `_authToken`, and `.properties` passwords. A value matched by several rules is reported once.
  - **v1 shims:** `ai-commit` with nothing to commit exits 0, like v1.

## [2.5.0] - 2026-10-10

Every AI tool picks up aitk without being told.

### Added
- **`aitk setup`** (once per machine, no project needed): a short marked section in each installed AI tool's user-level instructions (Claude Code, Codex, OpenCode, Gemini CLI, Qwen Code, Kiro, Windsurf) says that a repository with `aitk.toml` is managed by aitk and the agent starts with `aitk brief`, so agents follow aitk in every aitk repository, including fresh clones that were never synced. Also installs the Agent Skill for Claude Code at user level and registers the MCP server for Codex, OpenCode, and Gemini CLI. Backups first; `--dry-run`; `--remove` takes aitk's sections and entries out again (through symlinked dotfiles, keeping the link). Files that cannot be edited safely (unreadable, read-only, dangling symlinks, unbalanced markers, a hand-written file with aitk's name) are skipped and reported, never overwritten.
- **Adapters for more tools**: GitHub Copilot (`.github/copilot-instructions.md`, `.vscode/mcp.json`), Windsurf, Cline, Roo Code (with MCP), Aider (`read: [AGENTS.md]`), Junie, Qwen Code (with MCP). `aitk adapters sync --tool all` writes every tool's files for mixed teams; a problem with one tool's files (e.g. JSONC) skips that file instead of stopping the sync. Copilot is detected by its editor extension.

### Changed
- The MCP server's instructions and the `brief` tool tell agents to call `brief` first in every session without being asked, include `task_next` and `impact`, and say to ignore the tools in repositories without aitk.
- The user-level instructions say plainly that the agent finishes the task itself (`--ac-done`, then `aitk close`): a live test showed Claude Code stopping to ask for permission under the earlier wording.
- `aitk init` and the install script point to `aitk setup`; the README explains the four layers that make agents follow aitk.

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
