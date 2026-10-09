# Migrating v1 projects

`aitk migrate` converts a v1 project in place. It is additive and lossless: every v1 file it rewrites is first copied verbatim to `docs/ai/_legacy/`, and a migration report is written to `docs/ai/_legacy/MIGRATION.md`. `--dry-run` writes nothing and prints the same report.

A project is v1 when `ai-state.json` has `"version": 1` (or no `schema_version`), or `.ai-toolkit/project.env` exists without `aitk.toml`.

## Mapping

| v1 | v2 | Rule |
|---|---|---|
| `.ai-toolkit/project.env` | `aitk.toml` | `PROJECT_NAME` → `project.name` (slugified); `DEFAULT_BRANCH` → `project.default_branch`; `DEPLOY_DEFAULT/STAGING/PRODUCTION` → `deploy.targets.{default,staging,production}` when non-empty; `POST_DEPLOY_CHECK` → `deploy.post_check`; `UTEKE_NAMESPACE` → `plugins.settings.uteke.namespace`; `CODEBASE_PROJECT` → `plugins.settings.codebase-memory.project`. `PROJECT_ROOT` and `BROWSER_MCP_*` are dropped (machine-specific) and listed in the report. |
| `ai-state.json` v1 | `ai-state.json` v2 | `project.name` from config; `project.summary` from `context` unless it is the v1 placeholder. `modules`, `progress`, `bugs_found`, and `active_task` move to `_legacy/ai-state.json` (verbatim copy of the v1 file). |
| `docs/ai/current-task.md` (template form: has `Status:` and `Task ID:` lines) | `docs/ai/tasks/<id>-<slug>.md` | Title from the first line of `## Objective`; status mapped (below); `legacy.v1_id` keeps the v1 Task ID; `Goal ID` → `goal` when it matches `G-…`. Body sections are kept. |
| `docs/ai/current-task.md` (free form) | `_legacy/current-task.md` | Not converted; the report lists it and `aitk doctor` suggests `aitk task new`. |
| `docs/ai/tasks/<timestamp>.previous.md` | `docs/ai/tasks/<id>-<slug>.md` | Same rule as template-form `current-task.md`. |
| `docs/ai/tasks/<ID>-<name>.md` (hand-written, e.g. `F7-standardisasi-b8.md`) | same directory, renamed `<ID>-<slug>.md` | Converted only when all three hold: the filename prefix before the first `-` matches the ID pattern, the file has a `#` heading (title), and it has a `Status:` / `**Status:**` line (status); frontmatter is added, body kept verbatim. Files that cannot be parsed move to `_legacy/tasks/`. |
| `docs/ai/handoff.md` | `docs/ai/handoff/<UTC>-<task>.md` per `## ` section | `at` from a date or timestamp at the start of the heading, else from the previous section, else the file's last commit date; `outcome: legacy`; heading and body kept verbatim. Text before the first heading stays in `_legacy/handoff.md`. |
| `docs/ai/knowledge.md` | `docs/ai/knowledge/_inbox.md`, then compaction | Every top-level `- ` bullet (with its indented continuation lines) becomes an entry: `date` from the nearest preceding heading date, else the file's last commit date; `source=migrated`. |
| `docs/ai/decisions.md` | `docs/adr/<NNNN>-<slug>.md` per `## ` section | Numbered in file order after existing ADRs; `status: accepted`; `date` from the heading; body kept verbatim. |
| `AGENTS.md` block `<!-- ai-toolkit:protocol:start -->…<!-- ai-toolkit:protocol:end -->` | v2 block (workflow.md §2) | Replaced in place; text outside the block untouched. If the markers are missing, the v2 block is appended. |
| Other files in `docs/ai/` | unchanged | User content. |

### Status mapping

| v1 value (case-insensitive, emoji and trailing text ignored) | v2 |
|---|---|
| `ACTIVE`, `IN_PROGRESS`, `IN PROGRESS`, `EKSEKUSI` | `in_progress` |
| `IMPLEMENTED` | `implemented` |
| `REVIEW`, `REVIEWED`, `BUG_HUNT` | `in_review` |
| `COMPLETED`, `DONE`, `✅` | `done` |
| `BLOCKED` | `blocked` |
| `CANCELLED`, `CANCELED` | `cancelled` |
| `TODO`, `PLANNING`, anything else | `todo` (original kept in `legacy.status`) |

## Guarantees

- Lossless: every byte of every rewritten v1 file exists in `_legacy/`, and every knowledge bullet, handoff section, and decision section appears verbatim in its new file. The migration test suite asserts both.
- Idempotent: running `migrate` on a v2 project is a no-op.
- Portable: no absolute paths are written.
- All generated structured data validates against `schemas/`.
