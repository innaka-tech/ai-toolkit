---
status: accepted
date: 2026-10-09
---

# ADR-0002: Private agent coordination lives outside aitk

## Context and Problem Statement

v1 mixed a generic workflow with one user's private two-agent coordination (webhook reporter, remote handoff, network-specific protocol). The repository is public and meant for other users.

## Decision Outcome

Move the coordination layer to a separate private repository. aitk ships nothing user- or network-specific; such integrations are built as plugins (ADR-0007).

### Consequences

* Good: the public repository is generic and safe to publish.
* Good: private integrations evolve independently.
* Bad: users of the removed commands must install them from the private repository (done in v1.0.0, symlinked on PATH).
