---
description: Show what is ready to work on and start the next task
argument-hint: "[--goal G-x | --tag t]"
---
Run `aitk task next $ARGUMENTS` (MCP: `task_next`). Explain in one line each what is ready and what is waiting (and on what).
Then start the first ready task with `aitk task next --start $ARGUMENTS`, read its file, and begin. If the active task is unfinished, finish or hand it over first (`/aitk-close` or `/aitk-handover`).
