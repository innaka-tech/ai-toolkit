# Engineering: design and code

Code is written once and read for years: optimise for the next reader, the failure path, and the rollback. Follow the project's own conventions (`docs/ai/conventions.md`) first; this standard fills the gaps.

## Design
- One responsibility per module; dependencies point inward (domain does not import transport, UI, or framework code).
- Prefer boring, proven technology the team already runs. A new framework, database, or service needs an ADR with its TCO and exit path.
- Make illegal states unrepresentable: types, enums, and database constraints over comments and conventions.
- Idempotent operations at every boundary that can be retried (jobs, webhooks, payments, imports).
- Configuration from the environment, validated at startup; fail fast with a message that names the missing key. No secrets in code or in the repo.

## Code
- Match the surrounding style and the formatter/linter the project already uses; never reformat code you did not change.
- Functions do one thing; names say what, not how. No dead code, commented-out code, or unused parameters left behind.
- Handle every error once, at the level that can act on it: return it with context or handle it, never swallow it. User-facing errors say what happened and what to do next.
- No magic numbers or strings in logic: name them. Time is UTC in storage, zoned only at the edge. Money is integer minor units or a decimal type, never float.
- Concurrency: shared mutable state needs a lock, a queue, or a transaction; document which.
- Performance: measure before optimising; never put a query or network call inside a loop over unbounded data; paginate everything that can grow.

## Dependencies
- Add one only when the standard library and existing dependencies cannot do the job in reasonable code.
- Pin versions (lockfile committed), prefer maintained packages with a security policy, and check licences against the project's.
- Run `aitk audit` after adding or upgrading.

## Logging
- Structured (key=value or JSON), with a request or correlation id. Levels: error = someone must act, warn = degraded, info = business events, debug = off in production.
- Never log secrets, tokens, passwords, full card numbers, or personal data beyond what the purpose needs.
