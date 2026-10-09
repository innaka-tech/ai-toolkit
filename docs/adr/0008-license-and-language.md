---
status: accepted
date: 2026-10-09
---

# ADR-0008: Apache-2.0; English by default

## Context and Problem Statement

aitk should be adoptable by companies and individuals and accept outside contributions.

## Decision Outcome

License the project under Apache-2.0 (added with the first v2 release). CLI output, documentation, and generated templates are English by default; `AITK_LANG=id` selects Indonesian for CLI messages and templates.

### Consequences

* Good: permissive license with an explicit patent grant, common for developer tools.
* Good: English widens adoption; Indonesian remains first-class.
* Bad: every user-facing string needs a translation entry.
