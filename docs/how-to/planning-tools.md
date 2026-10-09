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
- **Superpowers**: `writing-plans` produces the plan → `aitk import superpowers` → `executing-plans` or `subagent-driven-development` works the steps, and `aitk close` verifies each task (check, criteria, review) before it counts as done.

The Agent Skill (`skills/aitk/SKILL.md`, installed for Claude Code by `aitk adapters sync`) tells agents to run this loop automatically.
