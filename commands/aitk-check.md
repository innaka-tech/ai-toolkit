---
description: Run the project's check and fix failures until it passes
---
Run `aitk check` (MCP: `check`). If it fails, read the output, fix the cause in the code (not the test, unless the test is wrong), and run it again.
If it fails twice for the same reason, stop: explain what you tried and run `aitk close --status blocked --summary "<what fails and why>" --knowledge none`.
When it passes, mark each acceptance criterion you satisfied: `aitk task update --ac-done N`.
