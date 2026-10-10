---
description: Check what the current change can break and cover it with tests
---
Run `aitk impact` (MCP: `impact`). For each changed file, look at the files that reference it and decide whether the change can break them.
- Where it says no test covers a changed file, add a focused test for the behaviour you changed.
- Where a deleted file is still referenced, fix the references.
Then run `aitk check` again. Report briefly: what could be affected, and which tests now cover it.
