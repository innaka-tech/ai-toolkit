# Let agents work through the backlog (aitk run)

`aitk run` takes the next ready task, starts it, and runs an AI agent on it without a person in the loop. It repeats until nothing is ready, a limit is reached, or the work gets stuck. aitk still decides what counts as done: a task finishes only when the agent's own `aitk close` passes the Definition of Done (passing check on the final code, every criterion met, the gates of its risk profile). aitk then verifies it again: the handoff that close wrote, the Definition of Done on the files as they are now, and the check, which aitk runs itself. A status the agent edited by hand is reopened.

```bash
aitk task next                      # what is ready, in order (dependencies done, not claimed elsewhere)
aitk run --dry-run --agent codex    # the order a run would take
aitk run --agent claude-code --commit --max-tasks 5
```

## What happens to each task

| Agent outcome | Task afterwards | The run |
|---|---|---|
| `aitk close` passes the Definition of Done, and aitk's own verification agrees | `done` (with `--commit`: one Conventional Commit with `AI-Task`/`AI-Tool` trailers) | next task |
| Needs review passes or a person's acceptance (strict tasks, `uat.required`) | `in_review`: a person runs `aitk uat accept <id>` | next task |
| The agent hands over with `aitk close --status blocked` | `blocked`, with the agent's handoff | next task |
| Not finished after `--max-attempts` runs (default 2), or the check keeps failing | `blocked`, with a handoff written by aitk (with `--commit`: its unfinished changes go to `git stash`, named after the task) | next task |
| The agent cannot start (exits at once without changing anything: not installed, bad flags, not logged in) | unchanged | the run stops |

The run stops after two unfinished tasks in a row (something is wrong that another attempt will not fix), when no task is ready, or after `--max-tasks` (default 10). A timeout (`--timeout`, minutes) ends the agent and every process it started. Only one run works in a worktree at a time; agents started by `aitk run` cannot start another run or accept UAT.

The run starts with this worktree's active task when there is one, then `in_progress` tasks, then `todo`, oldest first. Tasks created in the same second have no defined order: use `depends_on` (or Spec Kit's order) when order matters.

## Agents

| `--agent` | Command | Permissions by default |
|---|---|---|
| `claude-code` | `claude --permission-mode acceptEdits --allowedTools … -p` | read and edit files, run `aitk`, read-only `git` (validated) |
| `codex` | `codex exec --sandbox workspace-write` | Codex's workspace sandbox (validated) |
| `opencode` | `opencode run` | your OpenCode permission rules, which may allow any command |
| `gemini-cli` | `gemini --approval-mode auto_edit -p` | edits approved, other tools per Gemini CLI settings (not yet validated by us) |

The agent gets a one-line prompt pointing to the prompt file, so no shell or Windows `.cmd` shim parses the brief.

The defaults are deliberately narrow. If your agent needs more (for example `npm install`), grant it yourself in `aitk.toml`:

```toml
[run]
agent = "claude-code"
args = ["--allowedTools", "Bash(npm:*)"]
commit = true
max_tasks = 10
max_attempts = 2
timeout_minutes = 60
```

`[run].args` apply only to the agent configured in `[run].agent`. Any other tool: `aitk run --cmd "<command>"`. The prompt is on stdin and in `$AITK_RUN_PROMPT_FILE`; `$AITK_RUN_TASK` holds the task ID. `--commit` needs a clean working tree; `--commit=false` overrides `[run] commit = true`.

Run it on a branch or in a worktree (`aitk work <id>` for parallel runs), and review the commits before merging.

## Finding and fixing problems without a person

- `aitk audit --tasks` turns scanner findings into fix tasks: one per vulnerable dependency, titled with the version that fixes every advisory, and one per other failing scanner. A second audit does not duplicate open tasks. Follow it with `aitk run --tag security`.
- A bug fix (task tagged `bug`) cannot be done until a test file changed with it (in the task's own commits or uncommitted changes): the regression test that keeps the bug from coming back. Turn this off with `[quality] regression_tests = false`.
- `aitk impact` shows, for each changed source file, which files reference it and which tests cover it, and warns about changed files no test covers. The prompt of `aitk run` tells agents to run it before closing.

Together with the check on the final tree, the risk profiles, the security audit, and independent review for strict tasks, these are aitk's defences against a fix that breaks something else.

## How far the verification goes

Agents run as your user, so they can write any file in the repository, aitk's records included. `aitk run` therefore does not believe a task file that says `done`. It re-derives the outcome itself:

- The fields an agent must not change for itself go back to their values from before the attempt: user acceptance, review passes, audit results, regression evidence, the risk profile and its pin, tags, goal, and dependencies. Acceptance criteria may be added and checked, but a removed or reworded criterion fails the task.
- If the agent did not record a handoff with `aitk close`, aitk writes one, so every finished task has a record.
- aitk runs the check itself, and the audit too when the profile requires one.
- It recomputes the regression evidence from the task's own commits and uncommitted changes. Deleted tests do not count, and neither do other tasks' commits, which are recognized by their `AI-Task` trailer.
- It then applies the Definition of Done.

A task the agent claims but aitk cannot confirm is reopened.

This protects against sloppy and dishonest agents. It is not a sandbox:

- An agent that rewrites your test suite to pass trivially still passes the check. That is what review and `aitk impact` are for.
- A process that detaches into its own session can outlive the timeout.
- An agent that runs `git commit` itself bypasses `--commit`'s stash.

Run unattended work on a branch or in a worktree, use Codex's sandbox or Claude Code's permission rules, and review the commits before merging.

## What stays with a person

User acceptance (`aitk uat accept`), pushing and releasing (`aitk release --tag` never pushes), and anything an agent hands over as blocked.
