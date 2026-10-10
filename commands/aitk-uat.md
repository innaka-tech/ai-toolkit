---
description: Persona - prepare user acceptance testing for a task waiting for the user
argument-hint: "[task id]"
---
Prepare user acceptance for a person; you cannot accept on their behalf.

1. Find the task: the id given here (may be empty): $ARGUMENTS, or the one `aitk report` shows waiting in review.
2. Run `aitk uat script <id>`. It writes docs/ai/uat/<id>.md with one scenario per acceptance criterion.
3. Improve the scenarios where needed: concrete steps a non-developer can follow, the data to use, and the expected result for each.
4. Tell the user exactly what to do: follow the script, then run in their own terminal either `aitk uat accept <id>` or `aitk uat reject <id> --reason "…"`.

Never run `aitk uat accept` or `aitk uat reject` yourself.
