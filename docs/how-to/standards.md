# Development standards

aitk carries its development standards inside the binary, so every project and every agent gets the same rules without copying files, and personas can audit and correct code against them.

```bash
aitk standards              # list the topics
aitk standards security     # print one
aitk debt                   # deliberate shortcuts recorded in code
```

| Topic | Covers |
|---|---|
| `character` | How agents work: smallest-change ladder, never cut safety, correct instead of suggest, modes lite/full/ultra, output contract (Skipped / Risk) |
| `sdlc` | Phases, the persona for each, Definition of Ready and Done, change classes |
| `engineering` | Design, code, dependencies, logging |
| `testing` | What must be tested, test shape, edge-case checklist, coverage |
| `security` | Risk analysis (STRIDE), controls on OWASP Top 10 / ASVS L2, severity, incidents |
| `api` | Contracts (OpenAPI), RFC 9457 errors, pagination, idempotency, versioning |
| `database` | Integrity, migrations (expand/contract), queries, backup and restore |
| `operations` | Deploy, observability (RED/USE, SLO), backup and DR, change and incident, hardening |
| `git` | Branches, Conventional Commits, pull requests, SemVer, changelog |
| `review` | Review order, the finding format, independence and passes |
| `documentation` | Which document holds what, style |
| `ai-agents` | Coding agents' permissions and oversight; building LLM features (OWASP LLM Top 10) |
| `ux` | WCAG 2.1 AA and usability rules |

## Your own standards

A file `docs/ai/standards/<topic>.md` replaces the built-in topic of the same name, or adds a new one (for example `logistics.md` with your AWB and status rules). The first `# ` heading is the title and the first paragraph the summary. Topic names use lowercase letters, digits, and `-`. Commit the folder so the whole team and every agent use it.

Standards are guidance that personas check with evidence; the enforced gates stay in `aitk close`, the git hooks, and CI.

## Shortcuts as recorded debt

When the smallest working solution has a known limit, record it where it lives:

```go
// shortcut: in-memory cache, move to Redis when we run more than one instance
```

`aitk debt` lists every such comment (Go, Python, SQL, shell, HTML, … — not markdown) with file and line; `/aitk-auditor` includes them in its report.
