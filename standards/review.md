# Review and findings

A review finds defects that matter and makes each one actionable. Reviewing is not implementing: a reviewer does not start or close the task it reviews.

## What to look for, in order
1. Correctness against the acceptance criteria and the task's objective.
2. Security (see `aitk standards security`) and data integrity.
3. Failure handling: errors, retries, partial failure, rollback, concurrency.
4. Tests: does a test fail if the change breaks? Are edge cases covered?
5. Impact: code that references the change (`aitk impact`), compatibility of APIs, schemas, and configuration.
6. Operability: logs, metrics, migrations, configuration, documentation.
7. What to cut: code, dependencies, or features the need does not justify.

## Finding format (one block per finding)
```
[SEVERITY] file:line — title
What it does:   the current behaviour, concretely
What goes wrong: the input or condition and the observed or certain result
Fix:            the smallest correct change
Cost of skipping: what happens in production if this ships
```
Severity: critical, high, medium, low (definitions in `aitk standards security`). Only report what you reproduced or can show from the code; label anything else as a question.

## Not findings
Style the formatter or linter handles, personal preference, and "consider refactoring" without a concrete defect.

## Independence and passes
- Strict tasks need two review passes, the last with 0 findings, at least one by a different tool than the one that built it: `aitk review pass <id> --findings N`.
- Rotate the focus between passes: flow logic, edge cases, cross-module effects and permissions, security and data.
- Every fix re-runs the earlier passes' checks (regression).

## Turning findings into work
In scope of the current task: fix it test-first (`/aitk-fix`). Otherwise create a task with a reproducible criterion; the review report ends with the table of task ids.
