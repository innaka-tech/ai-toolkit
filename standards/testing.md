# Testing

Tests prove behaviour and protect it from regressions. The check command (`[check]` in aitk.toml) is the single source of truth; a task is done only when it passes on the final code.

## What must be tested
- Every acceptance criterion: at least one test that would fail if the criterion broke.
- Every bug fix: a regression test that fails before the fix (aitk refuses to close a bug task without a changed test).
- Non-trivial logic: branches, loops, parsers, money, dates and time zones, permissions, state transitions.
- Trust boundaries: invalid, missing, oversized, duplicate, and unauthorised input.
- Trivial changes (copy, config value, rename) need no new test.

## Shape
- Most tests are fast unit tests of behaviour, not implementation; integration tests cover each boundary (database, queue, external API through a fake or contract); a few end-to-end tests cover the critical user journeys.
- One behaviour per test; the name says the scenario and the expected result.
- Arrange-act-assert. No logic (loops, conditionals) in tests.
- Tests are deterministic: no real clock, network, or random seed without control; no order dependence. A flaky test is a bug: fix or quarantine it with a task, never retry it until green.
- Test data is built in the test (factories/builders), never copied from production.

## Edge-case checklist
Zero, negative, maximum, empty, null, unicode, very long input, duplicate submission, concurrent update, wrong order of operations, expired session, missing permission, partial failure of a multi-step operation, time-zone and daylight-saving boundaries, pagination limits.

## Coverage
Coverage is a hint, not a goal: aim for every risky path covered rather than a percentage. `aitk impact` lists changed code that no test references; cover it or say why not.

## Never
Skip, disable, or weaken a test to make the check pass. Mock the thing under test. Assert only that "no exception was thrown".
