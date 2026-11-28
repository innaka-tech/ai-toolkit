# Security, user acceptance, and conventions

## Configure

```toml
# aitk.toml
[risk]
sensitive_paths = ["src/auth/**", "migrations/**", "payments/**"]   # these make a task strict

[security]
asvs_level = 1        # OWASP ASVS 4.0.3 checklist for strict tasks (0 = off)
audit = "strict"      # security audit required before done: off | strict | all

[uat]
required = "strict"   # a person accepts before done: none | strict | standard | all
```

## A strict task, end to end

```bash
aitk task new "Refund payments" --ac "Given a paid order, when finance refunds it, then …" --start
# … work …
aitk check
aitk security checklist        # add the ASVS checklist; verify each item or write "N/A: <reason>" and check it
aitk audit                     # dependencies, static analysis, secrets
aitk review pass --findings 0  # twice, one by a tool that did not build it
aitk close --summary "…" --knowledge "…"   # → in_review: waiting for user acceptance
aitk uat script                # docs/ai/uat/<id>.md for the tester
```

Then the user, in their own terminal (agents are refused):

```bash
aitk uat accept T-k3m9 --note "refunded a test order in staging"   # → done
aitk uat reject T-k3m9 --reason "refund email shows the wrong amount" # → back to in_progress
```

The install script already installs [osv-scanner](https://google.github.io/osv-scanner/), which covers most ecosystems; on a machine that lacks it, run `aitk audit install` (Homebrew on macOS, else the checksum-verified release binary). `aitk doctor` warns when a project has dependency manifests and no scanner. Install semgrep for static analysis. Or set them explicitly:

```toml
[[security.scanners]]
name = "trivy"
cmd = "trivy fs --exit-code 1 --severity HIGH,CRITICAL ."
```

## Review until clean, one task at a time

```toml
[quality]
review = "standard"   # off | strict (default) | standard
one_task = true       # default
```

- `review = "standard"` (written by `aitk init` for new projects): a standard task is done only after a review pass finds 0 on the current code; a strict task also needs your own clean pass before another tool's. The agent reviews, records `aitk review pass --findings N`, fixes, and repeats; `aitk close` leaves the task `in_review` until then. Strict work still needs two passes, one clean (0 findings) by a tool that did not work on it. Lite work never needs the loop, but once a pass is recorded its findings must reach 0 before `done`.
- `one_task`: while the active task still needs its agent, `aitk task start` (and `task new --start`, `task next --start`) for another task fails with `E_TASK_ACTIVE`. Finish it, or park it with `aitk task block <id> --reason "…"`. A task that only waits for another tool's pass or a person's acceptance does not hold the worktree.
- When three or more passes stop improving, the brief tells the agent to stop and ask you.

Compatibility: aitk before 2.11 rejects `[quality] review` and `one_task` in `aitk.toml` (`E_SCHEMA`); everyone on the project needs 2.11 or later.

## Conventions

Fill `docs/ai/conventions.md` once: stack and versions, structure, naming, errors, data rules (money, time zones), tests, security, git, UI patterns. Every agent sees it in the brief, which keeps code consistent across tools and sessions.
