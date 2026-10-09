# Integrations specification

## Tool adapters

`aitk adapters sync` detects installed AI tools and installs three things per tool, where supported: instructions, MCP registration, and session hooks.

| Tool | Instructions | MCP registration | Session hook |
|---|---|---|---|
| Claude Code | `CLAUDE.md` starting with `@AGENTS.md` | `.mcp.json` (project) | `.claude/settings.json`: `SessionStart` (`startup\|resume\|compact`) → `aitk brief` |
| Codex CLI | `AGENTS.md` (native) | `~/.codex/config.toml` `[mcp_servers.aitk]`, only with `--global` (Codex has no project MCP config) | — |
| OpenCode | `AGENTS.md` (native) | `opencode.json` `mcp.aitk` (`type: local`) | — |
| Gemini CLI | `.gemini/settings.json` `context.fileName` includes `AGENTS.md` | `.gemini/settings.json` `mcpServers.aitk` | `SessionStart` (`startup`) → `aitk brief` |
| Kiro | `.kiro/steering/aitk.md` (`inclusion: always`) | `.kiro/settings/mcp.json` | — |
| Cursor | `.cursor/rules/aitk.mdc` (`alwaysApply: true`) | `.cursor/mcp.json` | — |
| Aider and others | `AGENTS.md` via the tool's read/include option | — | git hooks only |

Rules:

- Tools are detected by their executable on PATH or their project directory; `--tool` selects explicitly.
- Project files are written in the repository (reviewable in `git diff`, shared with teammates on commit). User-level files change only with `--global` and are copied to `$XDG_STATE_HOME/aitk/backups/<timestamp>/` first.
- JSON configs are edited key by key: other keys, their order, and their values are preserved; aitk only owns the `aitk` server entry and its own `aitk brief` hook. Files that are not plain JSON (e.g. JSONC with comments) are refused with an explanation, never rewritten.
- Text files: aitk writes only between its markers (`<!-- aitk:begin v=2 -->`, `# aitk:begin`) or files it owns (`.kiro/steering/aitk.md`, `.cursor/rules/aitk.mdc`).
- Re-running with no changes MUST be a no-op. `aitk adapters doctor` reports tools whose adapters are missing or outdated, and warns when `aitk` is not on PATH.
- Each adapter has fixture tests: existing user config, expected result, and a no-op re-run.
- Secrets are never written by adapters.
- `aitk mcp` starts even outside an aitk project, so a global registration is safe: tools then return `E_NOT_A_PROJECT` with its fix.

## MCP server

`aitk mcp` implements the [Model Context Protocol](https://modelcontextprotocol.io) (spec 2025-06-18) with the official Go SDK. Transport: stdio by default, streamable HTTP with `--http`.

| Kind | Name | Maps to |
|---|---|---|
| tool | `brief` | `aitk brief` |
| tool | `task_new`, `task_start`, `task_update`, `task_list`, `task_show` | `aitk task …` |
| tool | `check` | `aitk check` |
| tool | `close` | `aitk close` |
| tool | `switch_prepare` | `aitk switch --print` |
| tool | `knowledge_search`, `knowledge_add` | `aitk knowledge …` |
| tool | `adr_new` | `aitk adr new` |
| tool | `report` | `aitk report --format json` |
| tool | `doctor` | `aitk doctor` |
| resource | `aitk://brief`, `aitk://task/{id}` | brief and task content |
| prompt | `start-session`, `close-session` | the protocol steps as a prompt template |

- Tool input schemas are generated from the same structs as the CLI flags; tool results carry the `aitk.result/v1` envelope as structured content plus a text rendering.
- Validation errors return the error code and `fix` text, so a model can correct its call.
- MCP and CLI share one code path; their effects are identical.

## Gates

`aitk hooks install` adds an aitk block (between `# aitk:begin` and `# aitk:end`) to the `pre-commit`, `commit-msg`, and `pre-push` hooks in the effective hooks directory (`core.hooksPath` is respected). Existing hook content is kept and still runs; `aitk hooks uninstall` removes only the block. The block does nothing when `aitk` is not on PATH.

| Hook | Checks | On failure |
|---|---|---|
| pre-commit | Secrets in added lines (built-in rules for private keys, cloud/API tokens, JWTs, and high-entropy secret assignments; `aitk:allow-secret` marks a false positive); schema validation of staged `ai-state.json`, `aitk.toml`, task and handoff files; AGENTS.md block intact | block |
| commit-msg | [Conventional Commits 1.0](https://www.conventionalcommits.org) header (merge, revert, fixup, squash, and amend commits pass); trailers `AI-Task: <id>` and `AI-Tool: <tool>` added when a task is active | block |
| pre-push | `aitk check` passes when a task is active and a check is configured | block |

Humans can bypass with `--no-verify`; `aitk ci` runs the same gates in CI, where they cannot be bypassed: secrets and commit messages over `base..HEAD`, schema validation of changed files, and every `aitk doctor` error. The base comes from `--base`, `GITHUB_BASE_REF` / `CI_MERGE_REQUEST_TARGET_BRANCH_NAME`, or the merge base with the default branch.

GitHub Actions: `uses: innaka-tech/ai-toolkit/action@<ref>` (inputs `base`, `version`).

## Plugins

A plugin is any executable on PATH named `aitk-<name>` (ADR-0007).

1. **Discovery**: aitk lists PATH entries matching `aitk-*`. Only names in `plugins.enabled` (config) are called.
2. **Manifest**: `aitk-<name> manifest` prints an `aitk.plugin.manifest/v1` object (schemas/plugin.schema.json).
3. **Hook call**: `aitk-<name> hook <hook>` reads one `aitk.plugin.request/v1` object from stdin and prints one `aitk.plugin.response/v1` object to stdout.
4. **Limits**: `plugins.timeout` (default 5 s) per call; stdout capped at 1 MiB. Timeouts, crashes, and invalid output become warnings; they never fail the command.

| Hook | Called by | `payload` | `data` returned |
|---|---|---|---|
| `brief.sections` | `brief` | `{task, changed_paths, budget}` | `[{title, body, rank}]` |
| `close.after` | `close` (after writing) | `{task, handoff_path, knowledge}` | ignored |
| `doctor.checks` | `doctor` | `{}` | `[{name, status: ok\|warn\|error, detail}]` |
| `knowledge.search` | `knowledge search`, `brief` | `{query, limit}` | `[{text, score, source}]` |

Reference plugins (separate repositories): `aitk-uteke` (memory store), `aitk-codebase-memory` (code graph).
