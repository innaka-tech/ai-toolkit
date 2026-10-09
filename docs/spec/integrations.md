# Integrations specification

## Tool adapters

`aitk adapters sync` detects installed AI tools and installs three things per tool, where supported: instructions, MCP registration, and session hooks.

| Tool | Instructions | MCP registration | Session hooks |
|---|---|---|---|
| Claude Code | `CLAUDE.md` containing `@AGENTS.md`; skill `~/.claude/skills/aitk/SKILL.md` | `.mcp.json` (project) or user scope | `SessionStart` → `aitk brief`; `Stop` → remind to `aitk close` when the tree has changes |
| Codex CLI | `AGENTS.md` (native) | `~/.codex/config.toml` `[mcp_servers.aitk]` | when supported by the installed version |
| OpenCode | `AGENTS.md` (native) | `opencode.json` `mcp.aitk` | plugin event on session start |
| Gemini CLI | `.gemini/settings.json` `context.fileName` includes `AGENTS.md` | `.gemini/settings.json` `mcpServers.aitk` | when supported |
| Kiro | `.kiro/steering/aitk.md` | `.kiro/settings/mcp.json` | agent hooks |
| Cursor / Windsurf / GitHub Copilot | `AGENTS.md` or the tool's rules file | the tool's MCP config | — |
| Aider and others | `AGENTS.md` via the tool's read/include option | — | git hooks only |

Rules:

- Text files: aitk writes only between `<!-- aitk:begin v=2 -->` and `<!-- aitk:end -->`.
- JSON/TOML configs: aitk writes only keys it owns (the `aitk` server entry, the `aitk` hook entries) and preserves formatting where the format allows.
- The first run on a machine defaults to `--dry-run` and prints the diff. Every write is preceded by a backup in `~/.local/state/aitk/backups/<timestamp>/`.
- Re-running with no changes MUST be a no-op.
- Each adapter has fixture tests: a recorded "before" config, the expected "after", and a re-run that produces no change.
- Secrets are never written by adapters.

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

`aitk hooks install` uses the [pre-commit](https://pre-commit.com) framework when a `.pre-commit-config.yaml` exists, otherwise native git hooks (`core.hooksPath` is respected).

| Hook | Checks | On failure |
|---|---|---|
| pre-commit | Secret scan (gitleaks rules, embedded); schema validation of changed aitk files; AGENTS.md block intact | block |
| commit-msg | [Conventional Commits 1.0](https://www.conventionalcommits.org); trailers `AI-Task: <id>` and `AI-Tool: <tool>` added automatically when a task is active | block |
| pre-push | `aitk check` passes for the active task | block |

Humans can bypass with `--no-verify`; `aitk ci` runs the same gates in CI, where they cannot be bypassed. A reusable GitHub Action (`innaka-tech/ai-toolkit/action@v2`) and a GitLab CI template are provided.

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
