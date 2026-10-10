---
description: Finish the current task through aitk's Definition of Done
argument-hint: "[one-line summary]"
---
Finish the active task properly.

1. Run `aitk check` until it passes, and `aitk impact`; add tests where it says nothing covers a change. A task tagged `bug` needs a regression test.
2. Mark every satisfied criterion: `aitk task update --ac-done 1,2,…`.
3. Run `aitk close --summary "<what changed and why, one paragraph>" --knowledge "<one lasting finding for future sessions, or none>"` (MCP: `close`). Summary hint from the user, if any: $ARGUMENTS
4. If close refuses, do exactly what its `fix:` line says and close again. If it says the task waits for review or a person's acceptance, tell the user what they need to do (`aitk uat accept <id>` is theirs to run).

Never edit task status or evidence by hand.
