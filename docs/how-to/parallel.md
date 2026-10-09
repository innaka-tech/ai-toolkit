# Work in parallel

Several agents (or people) can work on different tasks at the same time.

```bash
aitk task new "Checkout" --ac "…"
aitk task new "Invoices" --ac "…"
git add -A && git commit -qm "chore: plan"

aitk work T-aaaa      # creates ../<repo>.aitk/T-aaaa on branch aitk/T-aaaa-checkout and starts the task there
aitk work T-bbbb
```

Open one agent per worktree. Each worktree has its own active task; `aitk task start` on a task claimed by another worktree fails with `E_TASK_CLAIMED` (exit 4). Claims expire after 4 hours and are released when the task is closed as done or blocked (`aitk task release <id> --force` clears a claim left by a deleted worktree).

Merge the branches as usual. Task and handoff files are one per entity, and `.gitattributes` (written by `aitk init`) merges knowledge files with git's `union` driver, so merges do not conflict on aitk files. Generated index files are rebuilt by the next aitk command (`aitk doctor --fix` rebuilds them on demand).
