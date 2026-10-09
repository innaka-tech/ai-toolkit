---
status: accepted
date: 2026-10-09
---

# ADR-0010: Ship v2.0 in English; Indonesian CLI output in v2.1

## Context and Problem Statement

ADR-0008 decided that CLI output and templates are English by default with `AITK_LANG=id` for Indonesian. v2.0 has about 60 user-facing messages and generated templates; translating them well needs a message catalog and review by a native speaker, which is not done.

## Decision Outcome

v2.0 ships English only. `AITK_LANG` is reserved and ignored. Indonesian output (message catalog with `golang.org/x/text/message`) is planned for v2.1. This supersedes the `AITK_LANG=id` part of ADR-0008; the rest of ADR-0008 stands.

### Consequences

* Good: v2.0 does not ship half-translated output.
* Bad: Indonesian-speaking users read English messages until v2.1. Error messages always include an exact `fix` command, which reduces the impact.
