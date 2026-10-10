---
description: Start a session in this aitk project - read the brief and take the next task
argument-hint: "[task id]"
---
Start working in this repository with aitk.

1. Run `aitk brief` (MCP: `brief`) and read all of it: active task, open criteria, last handoff, conventions, rules.
2. Task id given (may be empty): $ARGUMENTS. If there is one, run `aitk task start <id>`. Otherwise continue the active task, or run `aitk task next --start` (MCP: `task_next` with start=true) to take the first ready one.
3. Say in two or three lines which task you took, what "done" means for it (its acceptance criteria), and your first step. Then start working.

If nothing is ready, say so and suggest `/aitk-plan` to turn the next piece of work into tasks.
