# CLI specification

Follows [clig.dev](https://clig.dev). One binary, `aitk`, with subcommands.

## Global behavior

| Aspect | Rule |
|---|---|
| Project discovery | Walk up from the working directory to the git top level. Commands that need a project fail with `E_NOT_A_PROJECT` outside one. |
| Config precedence | built-in defaults < `~/.config/aitk/config.toml` < `aitk.toml` < `AITK_*` environment variables < flags |
| `--json` | Every command supports it and prints exactly one object matching `schemas/result.schema.json` to stdout. Human text then goes nowhere. |
| Streams | Results to stdout; progress, warnings, and errors to stderr. |
| Color | Only on a TTY; disabled by `NO_COLOR` or `--no-color`. |
| Prompts | Never prompt when stdin is not a TTY; `--yes` accepts defaults. Agents never see interactive prompts. |
| Errors | Every error states what failed and the exact command that fixes it (`error.fix` in JSON). |
| Language | English; `AITK_LANG=id` for Indonesian. |

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Runtime failure (I/O, git, check command failed) |
| 2 | Usage error (unknown command or flag, invalid argument) |
| 3 | Gate rejected (Definition of Done, schema validation, precondition) |
| 4 | Conflict (task claimed or locked by someone else) |

## Commands

### Project

| Command | Purpose |
|---|---|
| `aitk init [--name N] [--check CMD] [--dry-run]` | Create the layout, `aitk.toml`, the AGENTS.md block, and `ai-state.json`. Idempotent; never overwrites user content. |
| `aitk migrate [--dry-run] [--path P]` | Convert a v1 project (migration-v1.md). Prints a report; `--dry-run` writes nothing. |
| `aitk doctor [--fix]` | Validate schemas, AGENTS.md block, adapter drift, stale tasks (`in_progress` > 7 days), inbox size, plugin health. `--fix` repairs what is safe. |
| `aitk brief [--budget N] [--task ID]` | Print the brief (workflow.md §7). |

### Tasks

| Command | Purpose |
|---|---|
| `aitk task new "<title>" [--id ID] [--ac "<criterion>"]... [--tag T]... [--goal G] [--profile P]` | Create a task (`todo`). |
| `aitk task start <id>` | Set the session's active task; status → `in_progress`; claim it (§Parallel work). |
| `aitk task list [--status S] [--all-branches]` | List tasks. |
| `aitk task show <id>` | Print one task. |
| `aitk task update <id> [--status S] [--ac-done N] [--add-ac "<c>"] [--note "<text>"] [--tag T]` | Edit structured fields without hand-editing YAML. |
| `aitk task block <id> --reason "<text>"` | Status → `blocked`. |
| `aitk check` | Run `check.cmd` with `check.timeout`; record evidence on the active task. Exit 1 if the command fails. |
| `aitk close --summary "<text>" --knowledge "<text>\|none" [--next "<step>"]... [--status S]` | Apply the Definition of Done, write evidence, handoff, and knowledge. |
| `aitk review start\|pass --findings N\|done` | Record bug-hunt passes for strict tasks. |
| `aitk switch <tool> [--note "…"] [--print]` | Write a handoff (`outcome: switched`, `to: <tool>`) and a prompt file containing the brief; on a terminal start the tool with it (`claude`, `codex`, `gemini -i`, `opencode --prompt`), otherwise or with `--print` print how to. Unknown tools get the prompt file only, never a guessed command. |

### Parallel work

| Command | Purpose |
|---|---|
| `aitk task claim <id> [--ttl 4h]` | Record a claim in the shared git directory. Exit 4 if claimed by another session and not expired. |
| `aitk work <id>` | Create (or reuse) worktree `../<repo>.aitk/<id>` on branch `aitk/<id>-<slug>`, copy uncommitted aitk files it needs, claim the task, start it there. |
| (automatic) | `task start` claims the task for 4 h; `close` to done/blocked releases it. Knowledge and generated index files merge with git's `union` driver (`.gitattributes`), so parallel branches merge without conflicts. |
| `aitk task release <id>` | Drop the claim. |

### Knowledge, decisions, reporting

| Command | Purpose |
|---|---|
| `aitk knowledge add "<text>" [--tag T]... [--pin]` | Append to the inbox. |
| `aitk knowledge search "<query>" [--limit N]` | Ranked search (workflow.md §8). |
| `aitk knowledge compact [--dry-run]` | Run compaction. |
| `aitk knowledge pin <query>` | Pin matching entries. |
| `aitk adr new "<title>"` / `aitk adr list` | Create or list ADRs (MADR 4). |
| `aitk report [--since 7d] [--format md\|json]` | Done, in progress, and blocked tasks; who (which tool) did what; evidence links. |
| `aitk log [--task ID]` | Chronological handoffs. |

### Quality and delivery of the managed project

| Command | Purpose |
|---|---|
| `aitk audit` | Dependency vulnerabilities (osv-scanner, npm/pnpm audit, govulncheck, pip-audit, composer audit, cargo audit — whichever apply and are installed, or `security.scanners`), static analysis (semgrep OWASP Top 10 when installed), and a secret scan of tracked files; records evidence on the active task |
| `aitk security checklist [id]` | Add the OWASP ASVS 4.0.3 checklist to a task |
| `aitk uat script [id]` / `uat accept [id] [--by] [--note]` / `uat reject [id] --reason` | User acceptance (a person only) |
| `aitk goal add "<outcome>" [--id] [--parent]` / `goal list` / `goal status <id> <status>` | Goals with task progress |
| `aitk release [--bump] [--pre rc] [--dry-run] [--tag] [--skip-check]` | Version the project (SemVer, changelog, manifests, tag; never pushes) |

### Integration

| Command | Purpose |
|---|---|
| `aitk adapters sync [--tool T]... [--dry-run]` | Install instructions, MCP registration, and hooks for detected tools (integrations.md). |
| `aitk adapters doctor` | Report drift between installed and expected adapter content. |
| `aitk hooks install [--uninstall]` | Install git hooks (integrations.md §Gates). |
| `aitk ci` | Run every gate non-interactively; for CI pipelines. |
| `aitk mcp [--http :PORT]` | Serve MCP over stdio (default) or streamable HTTP. |
| `aitk import github-issues [--repo] [--label] [--state] [--dry-run]` | Issues become tasks `GH-<n>` (checkboxes → criteria, labels → tags, closed → done) via `gh`. |
| `aitk import spec-kit\|openspec [tasks.md…] [--dry-run]` | Checklist lines become tasks (`<FEATURE>-T001` keeps Spec Kit IDs); re-imports skip existing tasks. |
| `aitk plugin list` | Discovered `aitk-*` plugins, their hooks, and whether they are enabled. |
| `aitk deploy [target]` | Run `deploy.targets.<target>` then `deploy.post_check`; without a target, list targets. |
| `aitk version` | Version, commit, build date, supported schema versions. |

## Error codes

| Code | Exit | Meaning | `fix` example |
|---|---|---|---|
| `E_NOT_A_PROJECT` | 3 | Not inside an aitk project | `aitk init` |
| `E_NEEDS_MIGRATION` | 3 | v1 project detected | `aitk migrate --dry-run` |
| `E_NO_ACTIVE_TASK` | 3 | Command needs an active task | `aitk task start <id>` |
| `E_TASK_NOT_FOUND` | 2 | Unknown task ID | `aitk task list` |
| `E_TASK_CLAIMED` | 4 | Claimed by another session | `aitk task list` or wait for expiry |
| `E_LOCKED` | 4 | Another aitk process holds the lock | retry |
| `E_DOD_SECURITY` | 3 | Strict task without a verified OWASP ASVS checklist | `aitk security checklist <id>` |
| `E_DOD_AUDIT` | 3 | Security audit missing, failing, or stale | `aitk audit` |
| `E_AUDIT_FAILED` | 1 | A scanner or the secret scan found problems | fix and rerun `aitk audit` |
| `E_UAT_AGENT` | 3 | An AI agent tried to accept or reject on a person's behalf | the user runs `aitk uat accept <id>` |
| `E_SECRET` | 3 | Recorded text contains a credential | remove it |
| `E_RELEASE_NOTHING`, `E_RELEASE_DIRTY`, `E_RELEASE_CHECK` | 3 | Nothing releasable, uncommitted changes, failing check | commit work / `--bump`; commit or stash; fix the check |
| `E_STATE_UNWRITABLE` | 1 | Neither the git directory nor the working tree can hold aitk's private state | allow writes to the working tree |
| `E_CHECK_NOT_CONFIGURED` | 3 | `check.cmd` missing where required | `aitk init --check "<cmd>"` |
| `E_CHECK_FAILED` | 1 | Check command exited non-zero | fix and `aitk check` |
| `E_DOD_SUMMARY` | 3 | `--summary` missing | add `--summary` |
| `E_DOD_KNOWLEDGE` | 3 | `--knowledge` missing | add `--knowledge "…"` or `--knowledge none` |
| `E_DOD_CHECK_STALE` | 3 | No passing check for the current tree | `aitk check` |
| `E_DOD_ACCEPTANCE` | 3 | Criteria missing or unchecked | `aitk task update <id> --ac-done N` |
| `E_DOD_RISK` | 3 | Strict task without risk analysis | edit the Risk section |
| `E_SCHEMA` | 3 | A file fails its JSON Schema | `aitk doctor --fix` |
| `E_USAGE` | 2 | Invalid flags or arguments | `aitk <command> --help` |
| `E_GIT` | 1 | git failed | message includes git's stderr |
