# aitk

**Continuity and quality control for AI coding agents.**
aitk keeps a project's working state in git as plain files that every agent can read: Claude Code, Codex, OpenCode, Gemini CLI, Kiro, Cursor, Aider, or a human. Any agent can pick up where another stopped. Several agents can work in parallel without colliding. And a task is only *done* when a machine has verified it.

```bash
curl -fsSL https://raw.githubusercontent.com/innaka-tech/ai-toolkit/main/install.sh | sh
cd your-repo
aitk init              # AGENTS.md block, aitk.toml, docs/ai/
aitk adapters sync     # MCP server + session hook for the AI tools you have installed
aitk brief             # what every agent session starts with
```

## Why

| Problem | What happens without aitk | With aitk |
|---|---|---|
| Agents forget between sessions | Every session re-explains the project; switching tools starts from zero | `aitk brief`: one bounded summary (≤ 4k tokens by default) of the active task, last handoff, and relevant knowledge |
| Memory files rot | Notes grow without bound (we measured 180k tokens of "context" in one real project) | Knowledge is filed by topic, deduplicated, archived, and ranked by the files you are touching |
| "Done" isn't done | The agent says it's finished; tests are red, criteria unmet | `aitk close` refuses until the check passes on the current code and every acceptance criterion is satisfied |
| Parallel agents collide | Two agents edit the same task or files | Task claims, one worktree and branch per task, merge rules that keep parallel branches conflict-free |
| Rules in prompts get skipped | Long instructions are ignored on small tasks | Gates run in git hooks, CI, and the MCP server, not in the prompt |
| Every tool is configured differently | Instructions, MCP, and hooks set up by hand per tool | `aitk adapters sync` sets up every installed tool at once |

## How it works

```
1. aitk brief                  → the agent reads one bounded brief
2. aitk task start <id>        → or: aitk task new "<title>" --ac "Given …, when …, then …"
3. work; aitk check            → runs your test command, records evidence
4. aitk close --summary "…" --knowledge "…|none"
```

`close` applies the **Definition of Done** for the change's risk profile, chosen automatically from the diff:

| Profile | When | Requires |
|---|---|---|
| `lite` | ≤ 3 files and ≤ 100 lines | passing check on the final code, summary, knowledge; criteria optional, but any written must be met |
| `standard` | default | + every acceptance criterion satisfied |
| `strict` | touches `risk.sensitive_paths` (auth, migrations, infra…) | + risk analysis (STRIDE) + two review passes, the last with zero findings, one by a different tool |

When `close` refuses, the error says exactly what to run next (`fix: aitk task update T-k3m9 --ac-done 2`), so even small models recover on their own.

Everything is stored in your repository:

```
AGENTS.md                 # one aitk block; your own text stays untouched
aitk.toml                 # check command, sensitive paths, deploy targets, plugins
docs/ai/tasks/            # one file per task (YAML frontmatter, JSON Schema validated)
docs/ai/handoff/          # one file per session: who (which tool) did what, check result, next steps
docs/ai/knowledge/        # lasting findings, by topic
docs/adr/                 # architecture decisions (MADR)
```

No server, no account, works offline. The active task lives in the git directory of each worktree, never in a shared committed file.

## Works with your tools

| Tool | Instructions | MCP server | Session start |
|---|---|---|---|
| Claude Code | `CLAUDE.md` → `@AGENTS.md` | `.mcp.json` | `aitk brief` hook |
| Codex CLI | `AGENTS.md` | `~/.codex/config.toml` (`--global`) | |
| OpenCode | `AGENTS.md` | `opencode.json` | |
| Gemini CLI | `AGENTS.md` via settings | `.gemini/settings.json` | `aitk brief` hook |
| Kiro | steering file | `.kiro/settings/mcp.json` | |
| Cursor | always-on rule | `.cursor/mcp.json` | |
| anything else | `AGENTS.md` | | git hooks |

`aitk mcp` exposes the workflow as 15 MCP tools; every tool runs the same code as the CLI. Verified end-to-end with Claude Code and OpenCode.

## More than a task list

- **Parallel work**: `aitk work <id>` creates a worktree and branch, claims the task, and starts it there. Three agents on three tasks, merged back with zero conflicts, is part of the test suite.
- **Switch tools mid-task**: `aitk switch codex` writes a handoff and starts Codex with the brief.
- **Gates**: `aitk hooks install` adds a secret scan, schema validation, Conventional Commits with `AI-Task`/`AI-Tool` trailers, and a pre-push check. Your existing hooks keep running. `aitk ci` runs the same gates in CI (GitHub Action: `uses: innaka-tech/ai-toolkit/action@v2`).
- **Reports**: `aitk report --since 7d` shows what was done, by which tool, with evidence.
- **Imports**: GitHub Issues, Spec Kit, and OpenSpec task lists become aitk tasks.
- **Plugins**: any `aitk-<name>` executable on PATH can add to briefs, doctor checks, and knowledge search (JSON over stdin/stdout).
- **Migration from v1**: `aitk migrate` is lossless and idempotent. Every original file is kept byte for byte in `docs/ai/_legacy/`.

## Install

| Method | Command |
|---|---|
| Script (macOS, Linux) | `curl -fsSL https://raw.githubusercontent.com/innaka-tech/ai-toolkit/main/install.sh \| sh` (verifies SHA-256, and the cosign signature when cosign is installed) |
| Go | `go install github.com/innaka-tech/ai-toolkit/cmd/aitk@latest` |
| Manual / Windows | download from [Releases](https://github.com/innaka-tech/ai-toolkit/releases) and check `checksums.txt` |

One static binary, no runtime dependencies besides git.

## Documentation

- [Tutorial: your first project in 10 minutes](docs/tutorial.md)
- How-to: [set up AI tools](docs/how-to/ai-tools.md) · [work in parallel](docs/how-to/parallel.md) · [gates and CI](docs/how-to/ci.md) · [migrate from v1](docs/how-to/migrate-v1.md) · [write a plugin](docs/how-to/plugins.md)
- Reference: [CLI](docs/reference/cli.md) · [specification](docs/spec/README.md) · [JSON Schemas](schemas/)
- Explanation: [architecture decisions](docs/adr/README.md)

## Status

v2 is a rewrite of the v1 bash toolkit (still available at tag [`v1.0.0`](https://github.com/innaka-tech/ai-toolkit/releases/tag/v1.0.0); the `ai-*` commands keep working: they forward to `aitk` in v2 projects and run v1 unchanged in v1 projects).

Validated before release on 24 real v1 projects: all migrate losslessly, `aitk doctor` reports zero errors, every brief fits in 4k tokens and renders in about 100 ms. CI runs the full test suite with the race detector on Linux, macOS, and Windows.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues: [SECURITY.md](SECURITY.md). Licensed under [Apache-2.0](LICENSE).
