---
status: accepted
date: 2026-10-09
---

# ADR-0007: Plugins are executables speaking JSON

## Context and Problem Statement

Integrations (memory stores, code graphs, issue trackers, browsers) vary per user. Building them into the core makes it heavy and personal.

## Decision Outcome

A plugin is an executable named `aitk-<name>` on PATH (the git/kubectl model). aitk discovers it, asks for a manifest, and calls declared hooks with a JSON request on stdin, reading a JSON response from stdout. Plugin failures and timeouts are warnings, never fatal.

### Consequences

* Good: plugins can be written in any language and shipped independently.
* Good: the core stays small and offline.
* Bad: process spawn per hook call; mitigated by timeouts and calling plugins only from brief, close, doctor, and knowledge search.
