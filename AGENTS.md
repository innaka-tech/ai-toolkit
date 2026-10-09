# AGENTS.md

<!-- aitk:begin v=2 -->
## Working in this repository (aitk)

1. Run `aitk brief` and read its output. Do not read other files under docs/ai/ unless the brief points to them.
2. Run `aitk task next --start` (or `aitk task start <id>`) to take a task, or `aitk task new "<title>"` to create one.
3. Do the work. Run `aitk check` until it passes, and `aitk impact` to see what your change can break.
4. Run `aitk close --summary "<what changed>" --knowledge "<lasting finding, or none>"`.
5. If unsure, blocked, the change is risky, or the check fails twice: stop and ask the user.

If `aitk` is not installed, read docs/ai/project-context.md and the newest file in docs/ai/handoff/.
<!-- aitk:end -->
