---
status: accepted
date: 2026-10-09
---

# ADR-0003: Model selection, routing, and cost are out of scope

## Context and Problem Statement

v1 contained provider routing by keyword, account rotation, and budget settings that were never used. Model routing and cost control are solved by AI tools and OpenAI-compatible gateways.

## Decision Outcome

aitk is model- and vendor-neutral. It never selects, configures, or bills models. It works with any agent that can read files or run commands.

### Consequences

* Good: smaller scope; no drift with fast-moving provider APIs.
* Good: works identically with any tool, frontier or small model.
* Bad: cost reporting, if ever wanted, must come from a plugin.
