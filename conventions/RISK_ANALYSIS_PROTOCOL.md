# Risk Analysis Protocol

This protocol applies to every task, regardless of size or selected AI provider.

Before inspecting implementation details deeply or changing files, the active
AI must establish:

1. Objective and acceptance criteria.
2. Files, modules, interfaces, data, and users that may be affected.
3. Failure modes and edge cases, including invalid, missing, duplicate, stale,
   unauthorized, and concurrent input where relevant.
4. Security impact: trust boundaries, authentication, authorization, secrets,
   injection, data exposure, validation, and dependency risk.
5. Compatibility and consistency impact: API/schema/config/state transitions,
   provider behavior, persistence, and documentation.
6. Validation plan: focused tests, regression tests, static checks, and manual
   checks needed to prove the change safe.

Do not implement when a high-impact assumption is unverified. Record the
analysis and unresolved risks in `docs/ai/current-task.md`; resolve or explicitly
escalate them before marking the task implemented.

## Minimum task record

```text
Risk level: LOW | MEDIUM | HIGH | CRITICAL
Impact surface: <modules, interfaces, data, users>
Security concerns: <none, or concrete concerns>
Compatibility concerns: <none, or concrete concerns>
Failure modes/edge cases: <concrete cases>
Mitigations: <concrete controls>
Validation plan: <tests/checks>
Open risks/assumptions: <none, or explicit items>
```

Small task does not mean zero risk. It means analysis can be shorter when the
impact surface is demonstrably narrow.
