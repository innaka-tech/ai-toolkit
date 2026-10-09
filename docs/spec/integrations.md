# Integrations specification

## Tool adapters

`aitk adapters sync` detects installed AI tools and installs three things per tool, where supported: instructions, MCP registration, and session hooks.

| Tool | Instructions | MCP registration | Session hook |
|---|---|---|---|
| Claude Code | `CLAUDE.md` starting with `@AGENTS.md`; Agent Skill `.claude/skills/aitk/SKILL.md` | `.mcp.json` (project) | `.claude/settings.json`: `SessionStart` (`startup\|resume\|compact`) → `aitk brief` |
| Codex CLI | `AGENTS.md` (native) | `~/.codex/config.toml` `[mcp_servers.aitk]`, only with `--global` (Codex has no project MCP config) | — |
| OpenCode | `AGENTS.md` (native) | `opencode.json` `mcp.aitk` (`type: local`) | — |
| Gemini CLI | `.gemini/settings.json` `context.fileName` includes `AGENTS.md` | `.gemini/settings.json` `mcpServers.aitk` | `SessionStart` (`startup`) → `aitk brief` |
| Kiro | `.kiro/steering/aitk.md` (`inclusion: always`) | `.kiro/settings/mcp.json` | — |
| Cursor | `.cursor/rules/aitk.mdc` (`alwaysApply: true`) | `.cursor/mcp.json` | — |
| GitHub Copilot (VS Code) | `.github/copilot-instructions.md` (marked block) | `.vscode/mcp.json` `servers.aitk` (`type: stdio`) | — |
| Windsurf | `.windsurf/rules/aitk.md` (`trigger: always_on`) | — (user-level only) | — |
| Cline | `.clinerules/aitk.md`, or a marked block in a single `.clinerules` file | — | — |
| Roo Code | `.roo/rules/aitk.md` | `.roo/mcp.json` | — |
| Aider | `.aider.conf.yml` `read: [AGENTS.md]` (left alone when `read:` is set by hand) | — | — |
| Junie | `.junie/guidelines.md` (marked block) | — | — |
| Qwen Code | `.qwen/settings.json` `context.fileName` includes `AGENTS.md` | `.qwen/settings.json` `mcpServers.aitk` | — |
| Amp, Zed, Jules, Factory, others | `AGENTS.md` (native) | — | git hooks only |

Rule files for tools other than Claude Code, Codex, OpenCode, and Gemini carry one short pointer to the AGENTS.md block (`adapters.Pointer`), so the workflow has one source of truth. `--tool all` writes every tool's files regardless of detection.

Rules:

- Tools are detected by their executable on PATH, their project directory, or (Copilot, Cline, Roo Code) their editor extension; `--tool` selects explicitly.
- Project files are written in the repository (reviewable in `git diff`, shared with teammates on commit). User-level files change only with `--global` and are copied to `$XDG_STATE_HOME/aitk/backups/<timestamp>/` first.
- JSON configs are edited key by key: other keys, their order, and their values are preserved; aitk only owns the `aitk` server entry and its own `aitk brief` hook. Files that are not plain JSON (e.g. JSONC with comments) are refused with an explanation, never rewritten.
- Text files: aitk writes only between its markers (`<!-- aitk:begin v=2 -->`, `<!-- aitk:pointer:begin -->`, `<!-- aitk:global:begin -->`, `# aitk:begin`) or files it owns (`.kiro/steering/aitk.md`, `.cursor/rules/aitk.mdc`, `.windsurf/rules/aitk.md`, `.roo/rules/aitk.md`, `.clinerules/aitk.md`).
- Re-running with no changes MUST be a no-op. `aitk adapters doctor` reports tools whose adapters are missing or outdated, and warns when `aitk` is not on PATH.
- Each adapter has fixture tests: existing user config, expected result, and a no-op re-run.
- Secrets are never written by adapters.
- `aitk mcp` starts even outside an aitk project, so a global registration is safe: tools then return `E_NOT_A_PROJECT` with its fix.

## Machine setup

`aitk setup` (no project needed) makes every installed AI tool recognise aitk projects on its own, before any repository is configured:

| Tool | User-level instructions | Also |
|---|---|---|
| Claude Code | `~/.claude/CLAUDE.md` (marked block) | Agent Skill `~/.claude/skills/aitk/SKILL.md` |
| Codex CLI | `~/.codex/AGENTS.md` (marked block) | MCP in `~/.codex/config.toml` |
| OpenCode | `~/.config/opencode/AGENTS.md` (marked block) | MCP in `~/.config/opencode/opencode.json` |
| Gemini CLI | `~/.gemini/GEMINI.md` (marked block) | MCP in `~/.gemini/settings.json` |
| Qwen Code | `~/.qwen/QWEN.md` (marked block) | — |
| Kiro | `~/.kiro/steering/aitk.md` (owned file) | — |
| Windsurf | `~/.codeium/windsurf/memories/global_rules.md` (marked block) | — |

- The block (`adapters.GlobalText`) applies only in repositories with `aitk.toml`. It says to start with `aitk brief`, and it gives aitk's rules precedence over older ai-toolkit instructions.
- Only installed tools are touched; `--tool` selects explicitly; `--dry-run` shows the changes.
- Every edited file is backed up first.
- `--remove` takes the blocks, owned files (only if they are still aitk's), and MCP entries out again, and deletes files that held nothing else.
- Cursor and Copilot keep user-level instructions in their settings UI, not in files, so their repository files carry the workflow.

## MCP server

`aitk mcp` implements the [Model Context Protocol](https://modelcontextprotocol.io) (spec 2025-06-18) with the official Go SDK. Transport: stdio by default, streamable HTTP with `--http`.

| Kind | Name | Maps to |
|---|---|---|
| tool | `brief` | `aitk brief` (its description and the server instructions tell agents to call it first, unasked) |
| tool | `task_new`, `task_start`, `task_update`, `task_list`, `task_show` | `aitk task …` |
| tool | `check` | `aitk check` |
| tool | `task_next` | `aitk task next [--start]` |
| tool | `impact` | `aitk impact` |
| tool | `audit`, `security_checklist`, `uat_script` | `aitk audit [--tasks]`, `aitk security checklist`, `aitk uat script` |
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

## Planning tools

aitk executes and verifies plans made elsewhere; it does not replace planning frameworks.

| Source | Command | Mapping |
|---|---|---|
| BMAD Method tickets (`<type>-<slug>.md` with YAML frontmatter, type story/bug/spike) | `aitk import bmad [dir|file]` | id `BM-<EPIC>-<id>`; numbered Given/When/Then items (or the `Verify:` line) → criteria; status from the sibling `<type>-<slug>-plan.md` (draft/ready-for-dev → todo, in-progress, in-review/built → implemented, done, blocked, dropped → cancelled); `risk: high` → strict (pinned) |
| Superpowers plans (`docs/superpowers/plans/*.md`) | `aitk import superpowers [plan.md]` | each `### Task N: Name` → task `SP-<PLAN>-<N>`; its `- [ ] **Step k: …**` lines → criteria; all steps checked → done; `Files:` list → objective |
| Spec Kit / OpenSpec `tasks.md` | `aitk import spec-kit|openspec` | checklist lines → tasks; `T001` ids kept |
| Any markdown checklist | `aitk import markdown <file>` | checklist lines → tasks |
| GitHub Issues | `aitk import github-issues` | see the CLI reference |

Imports are idempotent: existing task ids are skipped. Imported tasks then follow aitk's Definition of Done.

The Agent Skill in `skills/aitk/SKILL.md` teaches agents the workflow (and when to stop); `aitk adapters sync` installs it for Claude Code.
