# Slash commands and personas

aitk installs the same commands into every AI tool you use, in each tool's own format. Agents also follow aitk on their own (see the README); commands are for when *you* want to steer: plan this, review that, hunt for bugs here.

| Command | What it does |
|---|---|
| `/aitk-start [id]` | Read the brief, take the next (or given) task, say what "done" means |
| `/aitk-plan <request>` | Split a request into tasks with Given/When/Then criteria, `bug` tags, and dependencies |
| `/aitk-next` | Show what is ready and what waits, then start the next task |
| `/aitk-check` | Run the check and fix failures; stop and hand over after two identical failures |
| `/aitk-impact` | See what the change can break and cover it with tests |
| `/aitk-close [summary]` | Finish through the Definition of Done |
| `/aitk-handover [tool]` | Hand over unfinished work with concrete next steps |
| `/aitk-status [7d]` | Done, in progress, blocked, waiting for you, ready next |
| `/aitk-review [id]` | **Reviewer persona**: independent bug hunt of a task's change; records `aitk review pass` (never starts the task, so the review stays independent) |
| `/aitk-hunt [area]` | **Bug hunter persona**: `aitk audit --tasks`, then proves defects and files each as a `bug` task |
| `/aitk-fix [id]` | **Fixer persona**: failing test first, smallest fix, impact check, close |
| `/aitk-uat [id]` | **UAT persona**: writes the acceptance script; tells you to run `aitk uat accept` |
| `/aitk-release [level]` | **Release manager persona**: next version and notes; tags only when you confirm, never pushes |

### SDLC personas

Each persona works in the aitk character (`aitk standards character`: smallest change that fully works, never cut safety, correct instead of only suggesting, end with **Skipped:** and **Risk:**) and checks the work against the matching standard (`aitk standards <topic>`). Personas that audit take a trailing `fix` to correct what they find in the same run; without it every finding becomes a task with a reproducible criterion.

| Command | Phase | What it does |
|---|---|---|
| `/aitk-team <request>` | any | **Team lead**: finds the SDLC phase from the evidence and runs the right persona, phase after phase, stopping for decisions that are yours |
| `/aitk-pm <idea>` | Discover | **Product manager**: problem, measurable outcome (`aitk goal add`), MoSCoW scope, hands the must items to planning |
| `/aitk-analyst <feature>` | Analyse | **Business analyst**: flow, state table, rules, edge cases, Given/when/then criteria written into tasks |
| `/aitk-architect <subject>` | Design | **Architect**: options with TCO and lock-in, Mermaid design, STRIDE summary, ADR; design defects become tasks |
| `/aitk-dba [scope] [fix]` | Design / Verify | **Database engineer**: constraints, migration safety (expand/contract, down path), indexes, N+1 |
| `/aitk-ux [scope] [fix]` | Design / Verify | **UX & accessibility**: WCAG 2.1 AA and usability findings, fixed or filed |
| `/aitk-qa [scope] [fix]` | Verify | **QA engineer**: coverage matrix (criterion/risk → test), missing tests added or filed |
| `/aitk-security [scope] [fix]` | Verify | **Security engineer**: `aitk audit --tasks`, threat model, OWASP review, proven findings fixed or filed |
| `/aitk-devops [scope] [fix]` | Release / Operate | **DevOps/SRE**: deploy, observability, backup and restore readiness; never touches production itself |
| `/aitk-auditor [topics] [fix]` | Operate | **Auditor**: scorecard of the project against every standard with evidence, ranked fix plan as tasks, report in `docs/ai/audit-<date>.md`; with `fix` closes the top quick wins |
| `/aitk-writer [scope]` | any | **Technical writer**: verifies documented commands and paths, corrects docs, records decisions as ADRs |
| `/aitk-caveman [lite\|full\|off]` | any | **Talk mode**: caveman replies (short, blunt, fewer tokens); code, commands, paths, errors, warnings, and Skipped/Risk stay exact |
| `/aitk-lean [lite\|full\|ultra\|off]` | any | **Character mode** for the session: lite proposes the smaller option, full builds the minimum, ultra challenges the request first |

The character is adapted from [Ponytail](https://github.com/DietrichGebert/ponytail) (MIT). Deliberate limits are recorded in code as `shortcut: <the limit>, <when to upgrade>` comments; `aitk debt` lists them.

A typical loop with several tools:

```
/aitk-plan add CSV export to reports    (Claude Code)
/aitk-next                              (Claude Code, or `aitk run` for all of them)
/aitk-review                            (Codex or OpenCode: a different tool, so the review counts as independent)
/aitk-uat                               then you run: aitk uat accept <id>
/aitk-release minor
```

## Where they are installed

| Tool | Built-in commands | Project commands | Invoke |
|---|---|---|---|
| Claude Code | `~/.claude/commands/` (`aitk setup`) | `.claude/commands/` | `/aitk-review` |
| Codex CLI | `~/.codex/prompts/` (`aitk setup`), plus the aitk skill in `~/.codex/skills/` | — | `/prompts:aitk-review` |
| OpenCode | `~/.config/opencode/commands/` (`aitk setup`) | `.opencode/commands/` | `/aitk-review` |
| Gemini CLI | `~/.gemini/commands/` (`aitk setup`) | `.gemini/commands/` | `/aitk-review` |
| Qwen Code | `~/.qwen/commands/` (`aitk setup`) | `.qwen/commands/` | `/aitk-review` |
| Cursor | `~/.cursor/commands/` (`aitk setup`) | `.cursor/commands/` | `/aitk-review` |
| pi | `~/.pi/agent/prompts/` (`aitk setup`), plus the aitk skill | — | `/aitk-review` |
| GitHub Copilot (VS Code) | `.github/prompts/*.prompt.md` (`adapters sync`) | same folder | `/aitk-review` |
| Windsurf | `.windsurf/workflows/` (`adapters sync`) | same folder | `/aitk-review` |
| any MCP client (jcode, Antigravity CLI, …) | MCP prompts (`aitk mcp`) | MCP prompts | e.g. `/mcp__aitk__aitk-review` |
| anything else | `aitk prompt aitk-review` prints the prompt | `aitk prompt <name>` | paste or pipe |

Built-ins go to user-level folders where a tool has one, so they do not appear twice in a repository that is also synced. Files aitk generates carry a "generated by aitk" note; aitk never overwrites a command of yours with the same name, and removes its own files when their source is gone.

## Your own commands

Write a markdown file in `docs/ai/commands/` and run `aitk adapters sync`:

```markdown
---
description: Verify a staging deploy before promoting it
argument-hint: "<version>"
---
Check the staging deploy of $ARGUMENTS:
1. Run `aitk check` and scripts/smoke.sh.
2. Compare the error rate with the previous version.
3. Report go / no-go with the evidence.
```

It becomes `/deploy-check` in every tool above (with `$ARGUMENTS` translated to each tool's placeholder), an MCP prompt, and `aitk prompt deploy-check v1.4`. Commit `docs/ai/commands/` and the generated files, and your whole team has it. A file named like a built-in (`aitk-review.md`) replaces that built-in for the project.

`aitk prompt <name> <text>` also makes any command scriptable:

```bash
aitk prompt aitk-hunt internal/payments | claude -p
aitk prompt aitk-review T-k3m9 | codex exec -
```
