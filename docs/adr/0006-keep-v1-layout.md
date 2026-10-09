---
status: accepted
date: 2026-10-09
---

# ADR-0006: Keep the v1 layout; add `aitk.toml`

## Context and Problem Statement

24 projects already use `AGENTS.md`, `ai-state.json`, and `docs/ai/*`, and agent instructions across tools refer to these paths.

## Decision Outcome

Keep `AGENTS.md`, `ai-state.json` (root, upgraded to schema version 2), and `docs/ai/`. Add structured subdirectories (`docs/ai/tasks/`, `docs/ai/handoff/`, `docs/ai/knowledge/`) and `docs/adr/`. Replace `.ai-toolkit/project.env` with `aitk.toml`; v2 still reads `project.env` when `aitk.toml` is absent. No absolute paths are stored in committed files.

### Consequences

* Good: migration is additive; existing instructions stay valid.
* Good: committed files are portable across machines (v1 stored machine-specific `Project Root` paths).
* Bad: `docs/ai/handoff.md` and `knowledge.md` become generated indexes instead of hand-edited logs.
