# Use aitk with BMAD, Superpowers, Spec Kit, or OpenSpec

Plan with the tool you like; execute and verify with aitk.

```bash
aitk import bmad                     # BMAD tickets anywhere in the repo (or give a folder/file)
aitk import superpowers              # docs/superpowers/plans/*.md
aitk import spec-kit                 # specs/*/tasks.md
aitk import openspec                 # openspec/changes/*/tasks.md
aitk import markdown notes/todo.md   # any checklist
aitk import github-issues --label ready
```

Every import has `--dry-run` and can be repeated: tasks already imported are skipped. What maps to what is in [integrations.md](../spec/integrations.md#planning-tools). BMAD tickets marked `risk: high` become strict tasks, so they get the security checklist, audit, and independent review.

Typical flows:

- **BMAD**: refine stories with BMAD's agents → `aitk import bmad` → agents work each `BM-…` task with `aitk brief` / `check` / `close` → `aitk release`.
- **Spec Kit**: `/speckit.specify` → `/speckit.plan` → `/speckit.tasks` → `aitk import spec-kit`. Phases and `[P]` markers become dependencies, so `aitk task next` (and `aitk run`) take tasks in Spec Kit's order and run parallel ones in any order; `[US1]` story markers become tags; each task links `spec.md` and `plan.md`. Closing a task checks off its line in `tasks.md`; importing again re-checks done tasks and reports lines checked only in the file (`checked_in_source_only`).
- **Superpowers**: `writing-plans` produces the plan → `aitk import superpowers` → `executing-plans` or `subagent-driven-development` works the steps, and `aitk close` verifies each task (check, criteria, review) before it counts as done.

The Agent Skill (`skills/aitk/SKILL.md`, installed for Claude Code by `aitk adapters sync`) tells agents to run this loop automatically.
