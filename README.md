# aitk

**Continuity and quality control for AI coding agents.**
aitk keeps a project's working state in git as plain files that every agent can read: Claude Code, Codex, OpenCode, Gemini CLI, Copilot, Cursor, Windsurf, Cline, Roo Code, Aider, Kiro, Junie, Qwen Code, or a human. Any agent picks up where another stopped, several agents work in parallel without colliding, and a task is *done* only when a machine has verified it. Agents work this way on their own: you don't have to explain aitk in your prompts.

```bash
curl -fsSL https://raw.githubusercontent.com/innaka-tech/ai-toolkit/main/install.sh | sh
aitk setup             # once per machine: your AI tools recognise aitk projects on their own
cd your-repo
aitk init              # AGENTS.md block, aitk.toml, docs/ai/
aitk adapters sync     # project files for the AI tools you use (commit them for your team)
```

Then just ask your agent for the work ("add a CSV export"). It reads the brief, takes a task, runs the check, and closes the task through the Definition of Done.

## Why

| Problem | Without aitk | With aitk |
|---|---|---|
| Agents forget between sessions | Every session re-explains the project; switching tools starts from zero | `aitk brief`: one bounded summary (≤ 4k tokens by default) of the active task, last handoff, conventions, and relevant knowledge |
| Memory files rot | Notes grow without bound (we measured 180k tokens of "context" in one real project) | Knowledge is filed by topic, deduplicated, archived, and ranked by the files you are touching |
| "Done" isn't done | The agent says it's finished; tests are red, criteria unmet | `aitk close` refuses until the check passes on the current code and every acceptance criterion is satisfied |
| Fixes break other things | A fix ships without a test; the bug returns, or something that depended on the code breaks | Bug fixes need a regression test; `aitk impact` shows what references the changed code and what no test covers |
| Parallel agents collide | Two agents edit the same task or files | Task claims, one worktree and branch per task, merge rules that keep parallel branches conflict-free |
| Rules in prompts get skipped | Long instructions are ignored on small tasks | Gates run in git hooks, CI, the MCP server, and `aitk run`, not in the prompt |
| Every tool is configured differently | Instructions, MCP, and hooks set up by hand per tool | `aitk setup` and `aitk adapters sync` configure every tool at once |

## How agents know what to do

Nobody has to tell an agent to use aitk. Four layers make sure it finds out:

| Layer | What it does | Set up by |
|---|---|---|
| **User-level instructions** | A short section in each AI tool's own user instructions says that a repository with `aitk.toml` is managed by aitk and the agent starts with `aitk brief`. It works in every aitk repository on the machine, including fresh clones. | `aitk setup` (once per machine; `--remove` undoes it) |
| **AGENTS.md** | The aitk block in the repository: five steps, shell commands only, read natively by most agents. | `aitk init` |
| **Tool-specific files** | For tools that don't read AGENTS.md by themselves: a rule or instruction file pointing to it, the MCP server, and a session hook that runs `aitk brief`. | `aitk adapters sync` |
| **MCP server** | 20 tools (`brief`, `task_next`, `check`, `impact`, `close`, …). Its instructions tell the agent to call `brief` first, without being asked. | registered by both commands above |

| Tool | Repository (`adapters sync`) | Machine (`setup`) |
|---|---|---|
| Claude Code | `CLAUDE.md` → `@AGENTS.md`, `.mcp.json`, SessionStart hook, Agent Skill | `~/.claude/CLAUDE.md`, user-level Agent Skill |
| Codex CLI | `AGENTS.md` (native) | `~/.codex/AGENTS.md`, MCP in `~/.codex/config.toml` |
| OpenCode | `AGENTS.md` (native), `opencode.json` MCP | `~/.config/opencode/AGENTS.md`, MCP in `opencode.json` |
| Gemini CLI | `.gemini/settings.json`: AGENTS.md as context, MCP, SessionStart hook | `~/.gemini/GEMINI.md`, MCP in `settings.json` |
| GitHub Copilot (VS Code) | `.github/copilot-instructions.md`, `.vscode/mcp.json` | |
| Cursor | `.cursor/rules/aitk.mdc` (always on), `.cursor/mcp.json` | |
| Windsurf | `.windsurf/rules/aitk.md` (always on) | `~/.codeium/windsurf/memories/global_rules.md` |
| Cline | `.clinerules` | |
| Roo Code | `.roo/rules/aitk.md`, `.roo/mcp.json` | |
| Kiro | `.kiro/steering/aitk.md`, `.kiro/settings/mcp.json` | `~/.kiro/steering/aitk.md` |
| Aider | `.aider.conf.yml`: `read: [AGENTS.md]` | |
| Junie | `.junie/guidelines.md` | |
| Qwen Code | `.qwen/settings.json`: AGENTS.md as context, MCP | `~/.qwen/QWEN.md` |
| Amp, Zed, Jules, Factory, others | `AGENTS.md` (native) | |
| anything else | `AGENTS.md`; git hooks enforce the rules for everyone | |

`adapters sync` configures the tools it detects; `--tool all` writes every tool's files, for teams whose members use different tools. Your own settings and text are kept: aitk changes only its own marked section or `aitk` entry (JSON files may be re-indented). A file it cannot edit safely (unreadable, read-only, JSON with comments, markers edited by hand) is skipped and reported, never overwritten.

## How it works

```
1. aitk brief                  → the agent reads one bounded brief
2. aitk task next --start      → or: aitk task new "<title>" --ac "Given …, when …, then …"
3. work; aitk check            → runs your test command, records evidence; aitk impact shows what the change touches
4. aitk close --summary "…" --knowledge "…|none"
```

`close` applies the **Definition of Done** for the change's risk profile, chosen automatically from the diff:

| Profile | When | Requires |
|---|---|---|
| `lite` | ≤ 3 files and ≤ 100 lines | passing check on the final code, summary, knowledge; criteria optional, but any written must be met |
| `standard` | default | + every acceptance criterion satisfied |
| `strict` | touches `risk.sensitive_paths` (auth, migrations, infra…) | + risk analysis (STRIDE), OWASP ASVS checklist, passing security audit, and two review passes (the last with zero findings, one by a tool that did not work on the task) |

On every profile, a task tagged `bug` needs a test file among its changes, and user acceptance by a person can be required (`uat.required`). When `close` refuses, the error says exactly what to run next (`fix: aitk task update T-k3m9 --ac-done 2`), so even small models recover on their own.

Everything is stored in your repository:

```
AGENTS.md                 # one aitk block; your own text stays untouched
aitk.toml                 # check command, sensitive paths, run settings, deploy targets, plugins
docs/ai/tasks/            # one file per task (YAML frontmatter, JSON Schema validated)
docs/ai/handoff/          # one file per session: who (which tool) did what, check result, next steps
docs/ai/knowledge/        # lasting findings, by topic
docs/ai/conventions.md    # your project's coding conventions, shown in every brief
docs/adr/                 # architecture decisions (MADR)
```

No server, no account, works offline. The active task lives in the git directory of each worktree, never in a shared committed file.

## Autopilot

```bash
aitk task next                                   # what is ready, in dependency order
aitk run --agent claude-code --commit --max-tasks 5
```

`aitk run` works through the ready tasks one at a time with a headless agent: `claude-code`, `codex`, `opencode`, `gemini-cli`, or any command (`--cmd`). aitk does not take the agent's word that a task is done. It restores what an agent must not change for itself (user acceptance, reviews, risk profile, tags, criteria), runs the check and any required audit itself, and applies the Definition of Done. Work that needs a person waits for them; unfinished work is handed over as blocked with a handoff (and, with `--commit`, stashed); the run stops when it gets stuck. See [how far the verification goes](docs/how-to/autopilot.md#how-far-the-verification-goes).

## More than a task list

- **Bug hunting**: `aitk audit` runs dependency scanners (osv-scanner, installed with aitk), static analysis, and a secret scan; `aitk audit --tasks` turns the findings into fix tasks, one per vulnerable package with the version that fixes it.
- **Parallel work**: `aitk work <id>` creates a worktree and branch, claims the task, and starts it there. Three agents on three tasks, merged back with zero conflicts, is part of the test suite.
- **Switch tools mid-task**: `aitk switch codex` writes a handoff and starts Codex with the brief.
- **Gates**: `aitk hooks install` adds a secret scan, schema validation, Conventional Commits with `AI-Task`/`AI-Tool` trailers, and a pre-push check. Your existing hooks keep running. `aitk ci` runs the same gates in CI (GitHub Action: `uses: innaka-tech/ai-toolkit/action@v2`).
- **Plans from other tools**: import Spec Kit (phases and `[P]` markers become dependencies; finished tasks are checked off in `tasks.md`), OpenSpec, BMAD, Superpowers, GitHub Issues, or any markdown checklist, then execute them with aitk's verification.
- **Delivery standards**: OWASP ASVS checklist for risky work, user acceptance by a person (`aitk uat accept`), `aitk release` for SemVer, changelog, and tags from Conventional Commits, project conventions, goals with progress.
- **Reports**: `aitk report --since 7d` shows what was done, by which tool, with evidence.
- **Plugins**: any `aitk-<name>` executable on PATH can add to briefs, doctor checks, and knowledge search (JSON over stdin/stdout).
- **Self-control**: aitk tells agents to stop after repeated failing checks, refuses credentials in anything it records, and keeps `done` behind the Definition of Done.
- **Self-healing**: hand-edited or conflicted aitk files are repaired before the next command (`self-healed:` warnings), and `aitk doctor --fix` restores drifted adapters and settings. Nothing is thrown away.
- **Migration from v1**: `aitk migrate` is lossless and idempotent. Every original file is kept byte for byte in `docs/ai/_legacy/`.

## Install

| Method | Command |
|---|---|
| Script (macOS, Linux) | `curl -fsSL https://raw.githubusercontent.com/innaka-tech/ai-toolkit/main/install.sh \| sh` (verifies SHA-256, and the cosign signature when cosign is installed) |
| Go | `go install github.com/innaka-tech/ai-toolkit/v2/cmd/aitk@latest` |
| Manual / Windows | download from [Releases](https://github.com/innaka-tech/ai-toolkit/releases) and check `checksums.txt` |

One static binary, no runtime dependencies besides git. The script also installs [osv-scanner](https://google.github.io/osv-scanner/) for `aitk audit` (skip with `AITK_NO_SCANNER=1`; elsewhere run `aitk audit install`). Then run `aitk setup` once.

## Documentation

- [Tutorial: your first project in 10 minutes](docs/tutorial.md)
- How-to: [set up AI tools](docs/how-to/ai-tools.md) · [autopilot: aitk run](docs/how-to/autopilot.md) · [security, UAT, conventions](docs/how-to/quality.md) · [release your project](docs/how-to/release.md) · [Spec Kit, BMAD, Superpowers](docs/how-to/planning-tools.md) · [work in parallel](docs/how-to/parallel.md) · [gates and CI](docs/how-to/ci.md) · [migrate from v1](docs/how-to/migrate-v1.md) · [write a plugin](docs/how-to/plugins.md)
- Reference: [CLI](docs/reference/cli.md) · [specification](docs/spec/README.md) · [JSON Schemas](schemas/) · [changelog](CHANGELOG.md)
- Explanation: [architecture decisions](docs/adr/README.md)

## Status

v2 is a rewrite of the v1 bash toolkit (still available at tag [`v1.0.0`](https://github.com/innaka-tech/ai-toolkit/releases/tag/v1.0.0); the `ai-*` commands keep working: they forward to `aitk` in v2 projects and run v1 unchanged in v1 projects).

- **v2 migration**: validated on 24 real v1 projects. All migrate losslessly, `aitk doctor` reports zero errors, and every brief fits in 4k tokens and renders in about 100 ms.
- **Autonomous runs**: validated live with Claude Code and Codex, which finished real tasks through `aitk run`, including bug fixes with regression tests.
- **Reviews**: the autonomous features went through three independent review rounds plus passes by Codex and OpenCode.
- **CI**: the full test suite runs with the race detector on Linux, macOS, and Windows.
- **Dogfooding**: aitk is developed with aitk.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues: [SECURITY.md](SECURITY.md). Licensed under [Apache-2.0](LICENSE).
