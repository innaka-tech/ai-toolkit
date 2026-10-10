---
description: Persona - fix a bug test-first and close it through the Definition of Done
argument-hint: "[bug task id]"
---
Fix a bug the safe way.

Bug task id given (may be empty): $ARGUMENTS

1. Take the bug task: `aitk task start <id>` with the id above, or `aitk task next --start --tag bug` when none was given. Read its objective and criteria.
2. Write a test that reproduces the bug and fails. Run `aitk check` and confirm it fails for the right reason.
3. Fix the cause with the smallest change that makes the test pass. Run `aitk check` until it passes.
4. Run `aitk impact`; cover anything that references the changed code and has no test.
5. Mark the criteria done (`aitk task update --ac-done N`) and finish with `aitk close --summary "<cause and fix>" --knowledge "<what caused it, so it does not happen again>"`.

aitk refuses to close a bug task without a changed test file.
