---
description: Plan - turn a request into aitk tasks with acceptance criteria and dependencies
argument-hint: "<what to build or fix>"
---
Plan this work as aitk tasks: $ARGUMENTS

1. Run `aitk brief` and read the conventions and existing open tasks, so you do not duplicate work. Look at the project's
   check command (`[check]` in aitk.toml) and its existing tests, so the plan fits the tools the project already uses.
2. Split the request into tasks a single session can finish (about half a day each). For each task:
   - a short imperative title;
   - 1-4 acceptance criteria as "Given …, when …, then …" that describe behaviour the user can observe, each checkable
     by the project's check. Do not prescribe a test framework, library, or file layout in a criterion;
   - tag `bug` for bug fixes (aitk then requires a regression test), `security` for security work;
   - dependencies on earlier tasks where order matters.
3. Create them in order with `aitk task new "<title>" --ac "…" [--ac "…"] [--tag bug] [--depends <id>]` (MCP: `task_new`). Use the ids it prints for later `--depends`.
4. Show the plan as a numbered list with ids, then run `aitk task next` to show what can start now.

Do not start implementing; planning ends when the tasks exist.
