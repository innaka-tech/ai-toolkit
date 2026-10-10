---
name: aitk
description: Work in a repository managed by aitk (it has aitk.toml and an aitk block in AGENTS.md). Use at the start of every session in such a repository, before finishing work, when importing a BMAD/Superpowers/Spec Kit plan, and when preparing a release.
---

# Working with aitk

aitk keeps this project's tasks, handoffs, and knowledge in git, and decides when work is done.
Use the `aitk` CLI (or the `mcp__aitk__*` tools); never edit files under docs/ai/ by hand.

## Every session

1. `aitk brief` — read it fully. It has the active task, open criteria, the last handoff, conventions, and rules.
2. Continue the active task, or take the next ready one with `aitk task next --start` (it respects dependencies),
   or `aitk task start <id>`, or `aitk task new "<title>" --ac "Given …, when …, then …" --start`.
3. Do the work following the Conventions section of the brief. Run `aitk check` until it passes.
   Before closing, run `aitk impact`: it lists the files that reference what you changed and the tests that cover them.
   Add tests where it says nothing covers a changed file. A bug fix (tag `bug`) needs a regression test, or close refuses.
4. Mark criteria you satisfied: `aitk task update --ac-done 1,2`.
5. `aitk close --summary "<what changed>" --knowledge "<lasting finding, or none>"`.
   If close refuses, run exactly the `fix:` command it prints.

## Stop and ask the user when

- `aitk check` has failed twice in a row (aitk says so), or you are blocked: hand over with
  `aitk close --status blocked --summary "<what you tried>" --knowledge "<what you learned>"`.
- The task is strict (auth, payments, migrations, infra): fill the Risk section, add the security checklist
  (`aitk security checklist`), verify each OWASP ASVS item, and run `aitk audit` before closing.
- The task waits for user acceptance: tell the user to run `aitk uat accept <id>` themselves.
  You cannot accept on their behalf; `aitk uat script <id>` writes the test script for them.

## Commands the user may type

`/aitk-plan`, `/aitk-next`, `/aitk-review`, `/aitk-hunt`, `/aitk-fix`, `/aitk-uat`, `/aitk-release`, and others (`aitk prompt` lists them,
including the project's own in docs/ai/commands/). Follow the command's steps; they use the same aitk commands as below.

## Finding and fixing problems

- `aitk audit --tasks` turns scanner findings into fix tasks: one per vulnerable dependency (with the fixed
  version), one per other failing scanner. Work them like any other task.
- Bug fixes: tag the task `bug`, write a test that fails without the fix, then fix it.

## Never

- Put secrets in summaries, knowledge, criteria, or notes (aitk refuses them).
- Mark work done with `task update`; only `close` can.
- Push or tag releases unless the user asks; `aitk release --dry-run` shows the next version.
- Start `aitk run`: it is the user's autonomous loop that starts agents like you (it refuses to nest).

## Plans from other tools

- BMAD tickets: `aitk import bmad` · Superpowers plans: `aitk import superpowers` ·
  Spec Kit / OpenSpec: `aitk import spec-kit|openspec` · any checklist: `aitk import markdown <file>`.
  Then work the imported tasks with the loop above; aitk adds verification on top of those workflows.
  Spec Kit phases and `[P]` markers become dependencies, so `aitk task next` gives the right order, and closing
  a task checks off its line in `tasks.md`.
