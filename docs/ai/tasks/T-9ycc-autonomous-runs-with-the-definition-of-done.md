---
id: T-9ycc
title: Autonomous runs with the Definition of Done
status: done
profile: strict
tags:
  - autopilot
created: "2026-10-09T16:54:39Z"
updated: "2026-10-09T18:46:06Z"
closed: "2026-10-09T18:46:06Z"
created_by: claude-code
workers:
  - claude-code
  - codex
branch: feat/autopilot
paths:
  - .claude/settings.json
  - .claude/skills/aitk/SKILL.md
  - .mcp.json
  - .omo/run-continuation/ses_ede0d2247ffeX12uzbXxtf4K7p.json
  - .omo/run-continuation/ses_ede25061affeg8q1WO06ECPbDy.json
  - CHANGELOG.md
  - README.md
  - docs/how-to/autopilot.md
  - docs/how-to/planning-tools.md
  - docs/reference/cli.md
  - docs/spec/cli.md
  - docs/spec/workflow.md
  - internal/agentsmd/agentsmd.go
  - internal/agentsmd/agentsmd_test.go
  - internal/audit/osv.go
  - internal/audit/osv_test.go
  - internal/cli/autopilot_test.go
  - internal/cli/cli.go
  - internal/doctor/doctor.go
  - internal/gates/gates.go
  - internal/gates/run_e2e_test.go
  - internal/mcpserver/server.go
  - internal/ops/audit_tasks.go
  - internal/ops/impact.go
  - internal/ops/importer.go
  - internal/ops/next.go
  - internal/ops/ops.go
  - internal/ops/proc_unix.go
  - internal/ops/proc_windows.go
  - internal/ops/quality.go
  - internal/ops/review_b_test.go
  - internal/ops/run.go
  - internal/project/project.go
  - internal/secrets/secrets_test.go
  - internal/task/task.go
  - schemas/config.schema.json
  - schemas/task.schema.json
  - skills/aitk/SKILL.md
review:
  passes:
    - "n": 1
      by: claude-code
      at: "2026-10-09T17:44:32Z"
      findings: 25
    - "n": 2
      by: claude-code
      at: "2026-10-09T17:44:33Z"
      findings: 11
    - "n": 3
      by: claude-code
      at: "2026-10-09T17:44:33Z"
      findings: 2
    - "n": 4
      by: codex
      at: "2026-10-09T17:46:49Z"
      findings: 1
    - "n": 5
      by: codex
      at: "2026-10-09T18:07:54Z"
      findings: 0
    - "n": 6
      by: opencode
      at: "2026-10-09T18:45:40Z"
      findings: 0
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 1089
    summary: |-
      …les]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/checkrun	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/claims	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/cli	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/compat	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/conv	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doc	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doctor	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/fsx	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/gates	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/gitx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/handoff	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/heal	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/knowledge	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/mcpserver	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/migrate	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/ops	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/plugins	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/profile	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/project	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/release	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/schema	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/secrets	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/session	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/task	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/textx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/schemas	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/skills	[no test files]
    at: "2026-10-09T18:46:03Z"
    tree: 0753d73004e790d273a13740a075fb90d56f3c16747f16c2fa1475334ba8b03f
  commits:
    - 287f1b1
    - 41a502d
    - 80ef0b3
    - 8af16af
    - 8f56e68
    - a4c7ae2
    - a7048ea
    - af7bdf6
    - b5580aa
    - db4dfcf
    - f7247d0
  acceptance:
    total: 6
    done: 6
  audit:
    at: "2026-10-09T18:46:03Z"
    passed: true
    tree: 0753d73004e790d273a13740a075fb90d56f3c16747f16c2fa1475334ba8b03f
    results:
      - name: osv-scanner (dependencies)
        exit_code: 0
        summary: |-
          Scanning dir .
          Starting filesystem walk for root: /
          Scanned /Users/anasfikri/.ai-toolkit/go.mod file and found 18 packages
          End status: 355 dirs visited, 1718 inodes visited, 1 Extract calls, 22.238208ms elapsed, 22.238ms wall time

          No issues found
      - name: aitk secret scan (tracked files)
        exit_code: 0
---

## Objective
(describe the outcome in one paragraph)

## Acceptance criteria
- [x] Given ready tasks, when aitk run runs a headless agent, then tasks finish only through the agent's aitk close
- [x] Given dependencies and claims, when aitk task next runs, then only startable tasks are listed in order
- [x] Given a failed audit, when aitk audit --tasks runs, then one fix task per vulnerable dependency exists without duplicates
- [x] Given a task tagged bug, when it closes without a changed test file, then close refuses
- [x] Given a Spec Kit tasks.md, when tasks are imported and closed, then phases become dependencies and lines are checked off
- [x] Given a change, when aitk impact runs, then referencing files and covering tests are listed

## Risk
Impact: new commands (run, task next, impact, audit --tasks/install) and a new DoD rule for tasks tagged bug; the AGENTS.md block gains a revision line, so projects see an outdated block until doctor --fix; the pre-commit gate now accepts other releases' blocks.
Security (STRIDE): Spoofing/Elevation — agents started by aitk run could forge done, UAT, reviews, profile, tags, or handoffs; mitigated by re-deriving the outcome from a pre-attempt snapshot plus aitk's own check/audit rerun, and AITK_RUN blocks uat accept. Tampering — --commit could sweep unrelated or secret files; mitigated by a clean-tree requirement, index reset, aitk-only staging, and stash. Denial of service — orphaned agent processes; process group killed on timeout and after each attempt (setsid escape documented). Information disclosure — prompt passed as a file (0600) in the git dir, not argv. osv-scanner download verified by SHA-256 from one pinned release.
Rollback: revert the merge commit; projects keep working with 2.3 (they may need doctor --fix to restore the old block); [run]/[quality] settings must be removed for older binaries.
Validation: unit and e2e tests for every review finding (forged done/UAT/profile/criteria, stash with renames, timeouts, locks, Spec Kit phases, audit IDs, regression gate scope); full suite with -race on macOS; Windows build/vet; live runs with Claude Code and Codex on a sample project including a bug task; three independent review rounds.

## Notes

## Security checklist (OWASP ASVS 4.0.3)
<!-- Check each item when verified, or write "N/A: <reason>" after the text and check it. -->
- [x] V1 Architecture: trust boundaries and the components handling sensitive data are identified for this change — boundary: agents started by aitk run are untrusted writers of the repo; decide() re-derives outcomes (see Risk)
- [x] V2 Authentication: credentials are verified server-side; no default, hard-coded, or weak secrets — N/A: no authentication; no secrets added (gitleaks clean)
- [x] V3 Session management: session tokens are random, protected, invalidated on logout and expiry — N/A: no sessions or tokens
- [x] V4 Access control: every endpoint/action checks authorization server-side, deny by default (no IDOR) — UAT accept refuses agents (AITK_RUN added to AgentInEnv); run refuses nesting and a second run (kernel lock)
- [x] V5 Validation, sanitization and encoding: all input validated; output encoded; no injection (SQL, OS, template) — prompt passed as a file (no shell/.cmd parsing); git pathspecs literal; syncSource rejects absolute/../non-md paths and symlinks leaving the repo; osv JSON parsed with typed structs
- [x] V6 Stored cryptography: sensitive data at rest uses approved algorithms; keys are not in the code — N/A: nothing encrypted; sha256 only for download integrity and ID hashing
- [x] V7 Error handling and logging: errors reveal no internals; security events are logged without secrets — typed E_* errors with fix lines; scanner output in tasks passes the credential guard (falls back to no output)
- [x] V8 Data protection: personal/sensitive data is minimized, not cached or logged, and removable — prompt file 0600 in the git dir; --commit refuses a dirty tree so local secrets are never swept into commits
- [x] V9 Communication: TLS for all external traffic; certificates validated — osv-scanner download over HTTPS from GitHub with default certificate validation, SHA-256 verified
- [x] V10 Malicious code: no backdoors, time bombs, or unexplained network calls; dependencies are trusted — only network call is aitk audit install (explicit); no new Go dependencies; govulncheck in CI
- [x] V11 Business logic: flows cannot be abused (limits, ordering, replay, race conditions) — forged done/UAT/profile/criteria/handoff all tested; max tasks/attempts, stop after two unfinished, lock against concurrent runs
- [x] V12 Files and resources: uploads are type/size limited and stored safely; no path traversal or SSRF — no uploads; download size capped (512 MB) and written via a temp file then renamed; path traversal checks in syncSource
- [x] V13 API and web service: API inputs/outputs are schema-validated; CORS and rate limits are deliberate — MCP inputs typed; config and task files schema-validated (new run/quality/regression_tests fields in schemas)
- [x] V14 Configuration: secure defaults, security headers, no debug in production, dependencies up to date — narrow default agent permissions; regression gate on by default; extra permissions only via explicit [run].args

## Evidence
<!-- Generated by aitk close. -->
- Check: `go vet ./... && go test -short ./...` → exit 0 at 2026-10-09T18:46:03Z (1089 ms)
- Acceptance criteria: 6/6 satisfied
- Commits: 287f1b1, 41a502d, 80ef0b3, 8af16af, 8f56e68, a4c7ae2, a7048ea, af7bdf6, b5580aa, db4dfcf, f7247d0
- Review pass 1 by claude-code: 25 findings
- Review pass 2 by claude-code: 11 findings
- Review pass 3 by claude-code: 2 findings
- Review pass 4 by codex: 1 findings
- Review pass 5 by codex: 0 findings
- Review pass 6 by opencode: 0 findings
