# Migrate a v1 project

```bash
aitk migrate --dry-run   # report only; writes nothing
aitk migrate
git diff --stat          # review
aitk doctor && aitk brief
git add -A && git commit -m "chore: migrate to aitk v2"
```

What happens is specified in [migration-v1.md](../spec/migration-v1.md). In short: every v1 file that is rewritten is first copied byte for byte to `docs/ai/_legacy/`; tasks, handoffs, knowledge, and decisions are converted; `docs/ai/_legacy/MIGRATION.md` lists warnings. Running `migrate` again is a no-op.

After migrating, set the check command in `aitk.toml` if it was not detected, then run `aitk adapters sync` and `aitk hooks install`. The v1 `ai-*` commands keep working: in a migrated project they forward to `aitk`.
