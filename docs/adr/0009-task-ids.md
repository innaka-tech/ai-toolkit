---
status: accepted
date: 2026-10-09
---

# ADR-0009: Short random task IDs

## Context and Problem Statement

Sequential IDs (`T-0001`) collide when tasks are created on parallel branches or machines. v1 used timestamps (`20260906-141827-358955`), which are unique but hard to read and type.

## Decision Outcome

Task IDs are `T-` followed by 4 lowercase Crockford base32 characters (e.g. `T-k3m9`), generated randomly and lengthened by one character on collision with an existing task. Users may supply their own ID (`--id F7`) matching `^[A-Za-z][A-Za-z0-9-]{0,31}$`.

### Consequences

* Good: collision-resistant without coordination; short enough to type and say.
* Good: imported tasks keep their original IDs.
* Bad: IDs carry no ordering; lists sort by `created` instead.
