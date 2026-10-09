# aitk v2 specification

Status: draft for v2.0. The key words MUST, MUST NOT, SHOULD, and MAY are used as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

aitk is a continuity and quality layer for software projects worked on by AI coding agents and humans. It stores project state in git as plain files that any agent can read. Any agent can resume another agent's work. Several agents can work in parallel without colliding. Every task finishes with machine-verified evidence.

| Document | Covers |
|---|---|
| [workflow.md](workflow.md) | Repository layout, agent protocol, risk profiles, Definition of Done, brief, knowledge |
| [cli.md](cli.md) | Commands, flags, JSON output, exit codes, error codes |
| [integrations.md](integrations.md) | Tool adapters, MCP server, git hooks and CI, plugins |
| [migration-v1.md](migration-v1.md) | Converting v1 projects |
| [../adr/](../adr/README.md) | Architecture decisions |
| [../../schemas/](../../schemas) | JSON Schemas (draft 2020-12) for every structured artifact |

## Design principles

1. **The tool does the work; the agent fills in text.** aitk writes all structured state. Agents call commands and supply short strings. This is what lets small models follow the workflow as reliably as large ones.
2. **Context has a budget.** Agents are never told to "read everything". `aitk brief` produces one bounded document; details are fetched on demand.
3. **Verified, not claimed.** A task is done when its check passes and its acceptance criteria are satisfied, recorded as evidence.
4. **Fail closed in machines, explain in prompts.** Gates live in the CLI, MCP server, git hooks, and CI.
5. **Git is the database** (ADR-0005). No server, no account, offline-first, merge-friendly.
6. **Small core, plugins for the rest** (ADR-0007).
7. **Neutral** toward tools, models, and vendors (ADR-0003).

## Non-goals

- Selecting, routing, or billing models.
- Hosting a server, dashboard, or SaaS.
- Being an agent. aitk complements agents; it does not replace them.
- Replacing spec-driven tools (Spec Kit, OpenSpec, Kiro specs) or issue trackers. aitk imports from and exports to them.
