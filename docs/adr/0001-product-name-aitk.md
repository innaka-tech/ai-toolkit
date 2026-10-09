---
status: accepted
date: 2026-10-09
---

# ADR-0001: Product and CLI name: `aitk`

## Context and Problem Statement

v1 ships ~30 `ai-*` executables plus an `ai-toolkit` dispatcher. Generic names like `ai-start` or `ai-status` collide easily with other tools on PATH, and the surface is hard to discover.

## Decision Outcome

Ship a single executable named `aitk` with subcommands (`aitk brief`, `aitk task start`, …). Keep `ai-toolkit` and the `ai-*` names as thin shims that forward to `aitk` for at least two minor releases after v2.0.

### Consequences

* Good: one name to install, document, and complete in shells; no PATH collisions.
* Good: existing projects and agent instructions keep working through shims.
* Bad: two names exist during the transition; shims must be removed deliberately (tracked in CHANGELOG).
